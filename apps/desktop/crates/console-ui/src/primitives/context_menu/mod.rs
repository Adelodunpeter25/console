//! A standalone, gpui-component-style context menu element.
//!
//! This component intentionally lives beside, rather than inside, the existing
//! dropdown menu primitive. It owns its `ContextMenuHandle` through GPUI
//! element state, so callers do not need to manage menu state themselves:
//!
//! ```ignore
//! div()
//!     .id("file-row")
//!     .context_menu(|_| {
//!         vec![MenuItem::new("Open", open_file)]
//!     })
//! ```
//!
//! Popup placement, focus restoration, keyboard handling, and dismissal are
//! delegated to the existing menu engine. Keeping those concerns shared avoids
//! having two subtly different context-menu implementations in the app.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Element, ElementId, GlobalElementId, InspectorElementId, InteractiveElement,
    IntoElement, ParentElement, StyleRefinement, Styled, Window,
};

use super::menu::{ContextMenuHandle, MenuItem, context_menu as render_context_menu};

pub mod session_context_menu;
pub use session_context_menu::session_context_menu;

/// Add a context menu to any interactive GPUI element.
pub trait ContextMenuExt: InteractiveElement + ParentElement + Styled {
    /// Attach a lazily-built context menu to this element.
    ///
    /// The builder runs only after the element is right-clicked. The element's
    /// stable ID is used for GPUI element state; when no ID is supplied, the
    /// call site is used instead, matching `gpui-component`'s behavior.
    #[track_caller]
    fn context_menu(
        mut self,
        builder: impl Fn(&mut App) -> Vec<MenuItem> + 'static,
    ) -> ContextMenu<Self>
    where
        Self: Sized,
    {
        let caller = std::panic::Location::caller();
        let id = self
            .interactivity()
            .element_id
            .clone()
            .map(|id| ElementId::Name(format!("context-menu-{id:?}").into()))
            .unwrap_or_else(|| ElementId::CodeLocation(*caller));

        ContextMenu::new(id, self, builder)
    }
}

impl<E: InteractiveElement + ParentElement + Styled> ContextMenuExt for E {}

/// A stateful context menu wrapper around a GPUI element.
pub struct ContextMenu<E> {
    id: ElementId,
    element: Option<E>,
    items: Rc<dyn Fn(&mut App) -> Vec<MenuItem>>,
    ignored_style: StyleRefinement,
}

impl<E: ParentElement + Styled> ContextMenu<E> {
    /// Construct a context menu with an explicit stable element ID.
    pub fn new(
        id: impl Into<ElementId>,
        element: E,
        builder: impl Fn(&mut App) -> Vec<MenuItem> + 'static,
    ) -> Self {
        Self {
            id: id.into(),
            element: Some(element),
            items: Rc::new(builder),
            ignored_style: StyleRefinement::default(),
        }
    }
}

impl<E: ParentElement + Styled> ParentElement for ContextMenu<E> {
    fn extend(&mut self, elements: impl IntoIterator<Item = AnyElement>) {
        if let Some(element) = &mut self.element {
            element.extend(elements);
        }
    }
}

impl<E: ParentElement + Styled> Styled for ContextMenu<E> {
    fn style(&mut self) -> &mut StyleRefinement {
        if let Some(element) = &mut self.element {
            element.style()
        } else {
            &mut self.ignored_style
        }
    }
}

impl<E: ParentElement + Styled + InteractiveElement + IntoElement + 'static> IntoElement
    for ContextMenu<E>
{
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

/// Persistent state for one `ContextMenu` element instance.
pub struct ContextMenuState {
    handle: Option<ContextMenuHandle>,
}

impl Default for ContextMenuState {
    fn default() -> Self {
        Self { handle: None }
    }
}

pub struct ContextMenuLayoutState {
    element: Option<AnyElement>,
}

impl<E: ParentElement + Styled + InteractiveElement + IntoElement + 'static> Element
    for ContextMenu<E>
{
    type RequestLayoutState = ContextMenuLayoutState;
    type PrepaintState = ();

    fn id(&self) -> Option<ElementId> {
        Some(self.id.clone())
    }

    fn source_location(&self) -> Option<&'static std::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        id: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (gpui::LayoutId, Self::RequestLayoutState) {
        let id = id.expect("ContextMenu must have a global element ID");

        window.with_optional_element_state::<ContextMenuState, _>(
            Some(id),
            |element_state, window| {
                let mut state = element_state.unwrap().unwrap_or_default();
                let handle = state
                    .handle
                    .get_or_insert_with(|| ContextMenuHandle::new(cx))
                    .clone();

                let element = self
                    .element
                    .take()
                    .expect("ContextMenu element should exist during layout");
                let items = self.items.clone();
                let mut element =
                    render_context_menu(element, self.id.clone(), &handle, move |cx| (items)(cx));
                let layout_id = element.request_layout(window, cx);

                let layout_state = ContextMenuLayoutState {
                    element: Some(element),
                };
                ((layout_id, layout_state), Some(state))
            },
        )
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        _: gpui::Bounds<gpui::Pixels>,
        request_layout: &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) {
        if let Some(element) = &mut request_layout.element {
            element.prepaint(window, cx);
        }
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        _: gpui::Bounds<gpui::Pixels>,
        request_layout: &mut Self::RequestLayoutState,
        _: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        if let Some(element) = &mut request_layout.element {
            element.paint(window, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn context_menu_state_starts_without_runtime_entities() {
        let state = ContextMenuState::default();
        assert!(state.handle.is_none());
    }
}
