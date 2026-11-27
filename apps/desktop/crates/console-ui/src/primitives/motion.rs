//! Shared pulse clock for the repeating loaders.
//!
//! Ported from Zeron's motion kit (<https://github.com/zeronsh/comet>, MIT).
//! A repeating `with_animation` element requests a redraw every display frame
//! for as long as it is mounted — one working row pinned the whole window at
//! 120 Hz on a ProMotion panel. Loaders instead read their phase from one
//! shared clock: it ticks at ~30 fps, notifies only the tiny `PulseView`
//! entity that owns the loader (so `gpui-fast` splices just the spinner
//! without rebuilding the parent view), and parks itself once the last lease
//! lapses.

use std::rc::Rc;
use std::sync::LazyLock;
use std::time::{Duration, Instant};

use gpui::{
    AnyElement, App, AppContext, Context, ElementId, Entity, EntityId, Global, Hsla, IntoElement,
    Render, Transformation, Window, percentage,
};

use super::{IconName, app_icon};

/// Repeat-tick interval (~30 fps): visually equivalent for these chunky
/// pulses and spins at a quarter of a ProMotion display's redraws.
const PULSE_TICK: Duration = Duration::from_millis(33);

/// How long a view stays on the tick list after it last painted a loader. One
/// lease outlives a few missed frames; an unmounted loader stops renewing and
/// its view drops off, letting the clock park.
const PULSE_LEASE: Duration = Duration::from_millis(300);

/// The rotating `loader-circle` spinners' period.
const SPINNER_PERIOD: Duration = Duration::from_millis(900);

static PULSE_EPOCH: LazyLock<Instant> = LazyLock::new(Instant::now);

struct Lease {
    until: Instant,
    stride: u32,
}

#[derive(Default)]
struct PulseClock {
    leases: Vec<(EntityId, Lease)>,
    ticks: u64,
    running: bool,
}

impl Global for PulseClock {}

/// Keep `view` re-rendering at [`PULSE_TICK`] until the lease lapses. A caller
/// that stops leasing stops being notified, and the clock parks once no
/// leases remain — quiescence needs no unsubscribe step.
pub fn pulse_lease(view: EntityId, cx: &mut App) {
    pulse_lease_with_stride(view, 1, cx);
}

/// [`pulse_lease`] at every second tick (~15 fps).
pub fn pulse_lease_slow(view: EntityId, cx: &mut App) {
    pulse_lease_with_stride(view, 2, cx);
}

fn pulse_lease_with_stride(view: EntityId, stride: u32, cx: &mut App) {
    let clock = cx.default_global::<PulseClock>();
    let until = Instant::now() + PULSE_LEASE;
    if let Some((_, lease)) = clock.leases.iter_mut().find(|(id, _)| *id == view) {
        lease.until = until;
        lease.stride = lease.stride.min(stride);
    } else {
        clock.leases.push((view, Lease { until, stride }));
    }
    if clock.running {
        return;
    }
    clock.running = true;
    cx.spawn(async move |cx| {
        loop {
            cx.background_executor().timer(PULSE_TICK).await;
            let parked = cx.update(|cx| {
                let clock = cx.default_global::<PulseClock>();
                let now = Instant::now();
                clock.ticks += 1;
                let ticks = clock.ticks;
                clock.leases.retain(|(_, lease)| lease.until > now);
                if clock.leases.is_empty() {
                    clock.running = false;
                    return true;
                }
                let mut due = [None; 16];
                let mut due_count = 0;
                for (view, lease) in clock.leases.iter_mut() {
                    if ticks % lease.stride.max(1) as u64 == 0 {
                        lease.stride = u32::MAX;
                        if due_count < due.len() {
                            due[due_count] = Some(*view);
                            due_count += 1;
                        }
                    }
                }
                for view in due.into_iter().take(due_count).flatten() {
                    cx.notify(view);
                }
                false
            });
            if parked {
                break;
            }
        }
    })
    .detach();
}

fn pulse_phase(period: Duration, stride: u32, view: EntityId, cx: &mut App) -> f32 {
    if cx.reduce_motion() {
        return 0.0;
    }
    let phase = (PULSE_EPOCH.elapsed().as_secs_f32() / period.as_secs_f32()).fract();
    cx.defer(move |cx| pulse_lease_with_stride(view, stride, cx));
    phase
}

struct PulseView {
    period: Duration,
    stride: u32,
    render: Rc<dyn Fn(f32) -> AnyElement>,
}

impl Render for PulseView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let phase = pulse_phase(self.period, self.stride, cx.entity_id(), cx);
        (self.render)(phase)
    }
}

/// A loader element styled from the shared clock's phase, isolated in its own
/// `Entity` view so ticks only splice the loader instead of rebuilding its
/// parent view.
#[track_caller]
pub fn pulse(period: Duration, render: impl Fn(f32) -> AnyElement + 'static) -> Pulse {
    Pulse {
        key: ElementId::CodeLocation(*core::panic::Location::caller()),
        period,
        stride: 1,
        render: Rc::new(render),
    }
}

/// A rotating loader icon riding the shared clock.
#[track_caller]
pub fn spin(icon: IconName, size: f32, color: Hsla) -> AnyElement {
    spin_with_stride(
        ElementId::CodeLocation(*core::panic::Location::caller()),
        icon,
        size,
        color,
        1,
    )
}

/// A rotating loader at every second tick (~15 fps).
#[track_caller]
pub fn spin_slow(icon: IconName, size: f32, color: Hsla) -> AnyElement {
    spin_with_stride(
        ElementId::CodeLocation(*core::panic::Location::caller()),
        icon,
        size,
        color,
        2,
    )
}

fn spin_with_stride(
    key: ElementId,
    icon: IconName,
    size: f32,
    color: Hsla,
    stride: u32,
) -> AnyElement {
    Pulse {
        key,
        period: SPINNER_PERIOD,
        stride,
        render: Rc::new(move |phase| {
            app_icon(icon, size, color)
                .with_transformation(Transformation::rotate(percentage(phase)))
                .into_any_element()
        }),
    }
    .into_any_element()
}

#[derive(IntoElement)]
pub struct Pulse {
    key: ElementId,
    period: Duration,
    stride: u32,
    render: Rc<dyn Fn(f32) -> AnyElement>,
}

impl Pulse {
    /// Tick every `stride`-th pulse instead of every one.
    pub fn every(mut self, stride: u32) -> Self {
        self.stride = stride.max(1);
        self
    }
}

impl gpui::RenderOnce for Pulse {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let period = self.period;
        let stride = self.stride;
        let render = self.render;
        window.with_global_id(self.key, |global_id, window| {
            window.with_element_state(global_id, |state: Option<Entity<PulseView>>, _| {
                let entity = match state {
                    Some(entity) => {
                        entity.update(cx, |view, _| {
                            view.period = period;
                            view.stride = stride;
                            view.render = render;
                        });
                        entity
                    }
                    None => cx.new(|_| PulseView {
                        period,
                        stride,
                        render,
                    }),
                };
                (entity.clone(), entity)
            })
        })
    }
}
