//! Microphone capture for speech-to-text dictation. Pure client-side: records
//! into memory, hands the WAV bytes to the caller, and drops everything when
//! the call ends — nothing touches disk.

use std::sync::{Arc, Mutex};

use anyhow::{Context, Result, bail};
use cpal::traits::{DeviceTrait, HostTrait, StreamTrait};
use cpal::{SampleFormat, Stream, SupportedStreamConfig};

/// The sample rate the server expects whisper input at. Whisper resamples
/// internally, but sending 16 kHz mono keeps a 2-minute dictation at ~3.8 MB —
/// under both the Whisper input cap and the server's 4 MB upload cap, which
/// a native 48 kHz stereo stream would blow past in ~20 seconds.
pub const TARGET_SAMPLE_RATE: u32 = 16_000;

/// Recording longer than this is pointless: at 16 kHz mono 16-bit it would
/// exceed the server's 4 MB audio cap.
pub const MAX_RECORD_SECONDS: u64 = 120;

pub struct Recorder {
    // Held only to keep the stream alive; dropped with the recorder.
    _stream: Stream,
    samples: Arc<Mutex<Vec<f32>>>,
    source_rate: u32,
    error: Arc<Mutex<Option<String>>>,
}

impl Recorder {
    /// Open the default input and start capture. On macOS the first open
    /// raises the system microphone-permission prompt; if the user denies
    /// it, opening the device fails and the error names the fix.
    pub fn start() -> Result<Self> {
        let host = cpal::default_host();
        let device = host
            .default_input_device()
            .context("No microphone found on this device.")?;
        let config: SupportedStreamConfig = device
            .default_input_config()
            .context("Could not read the microphone's default format.")?;

        let source_rate = config.sample_rate().0;
        let channels = config.channels();
        let samples: Arc<Mutex<Vec<f32>>> = Arc::new(Mutex::new(Vec::new()));
        let error: Arc<Mutex<Option<String>>> = Arc::new(Mutex::new(None));

        let stream = match config.sample_format() {
            SampleFormat::F32 => {
                let samples = samples.clone();
                let error = error.clone();
                device.build_input_stream(
                    &config.into(),
                    move |data: &[f32], _| push_mono(&samples, data.iter().copied(), channels),
                    move |err| *error.lock().unwrap() = Some(err.to_string()),
                    None,
                )
            }
            SampleFormat::I16 => {
                let samples = samples.clone();
                let error = error.clone();
                device.build_input_stream(
                    &config.into(),
                    move |data: &[i16], _| {
                        push_mono(&samples, data.iter().map(|&s| f32::from(s) / 32768.0), channels)
                    },
                    move |err| *error.lock().unwrap() = Some(err.to_string()),
                    None,
                )
            }
            SampleFormat::U16 => {
                let samples = samples.clone();
                let error = error.clone();
                device.build_input_stream(
                    &config.into(),
                    move |data: &[u16], _| {
                        push_mono(&samples, data.iter().map(|&s| f32::from(s) / 32768.0 - 1.0), channels)
                    },
                    move |err| *error.lock().unwrap() = Some(err.to_string()),
                    None,
                )
            }
            _ => {
                bail!("Microphone uses an unsupported sample format.")
            }
        }
        .context("Could not start recording (microphone access denied?)")?;

        stream.play().context("Could not start recording.")?;

        Ok(Self {
            _stream: stream,
            samples,
            source_rate,
            error,
        })
    }

    /// Stop capture and encode the take as a 16 kHz mono 16-bit WAV.
    pub fn stop_into_wav(self) -> Result<Vec<u8>> {
        drop(self._stream);
        if let Some(err) = self.error.lock().unwrap().take() {
            bail!("Recording stopped early: {err}");
        }
        let samples = std::mem::take(&mut *self.samples.lock().unwrap());
        let mono16 = resample_to(&samples, self.source_rate, TARGET_SAMPLE_RATE);
        write_wav(&mono16)
    }
}

/// Downmix interleaved frames to mono in the capture callback. The callback
/// runs on the capture thread, so this must stay allocation-free apart from
/// the push.
fn push_mono(
    samples: &Arc<Mutex<Vec<f32>>>,
    frames: impl Iterator<Item = f32>,
    channels: u16,
) {
    let channels = channels.max(1) as usize;
    let mut buf = samples.lock().unwrap();
    for (i, s) in frames.enumerate() {
        if i % channels == 0 {
            buf.push(s);
        }
    }
}

/// Linear-interpolation resample to `target` Hz. Input is native-rate mono.
fn resample_to(samples: &[f32], source_rate: u32, target: u32) -> Vec<i16> {
    if samples.is_empty() || source_rate == 0 {
        return Vec::new();
    }
    if source_rate == target {
        return samples
            .iter()
            .map(|&s| (s.clamp(-1.0, 1.0) * 32767.0) as i16)
            .collect();
    }
    let ratio = f64::from(source_rate) / f64::from(target);
    let out_len = (samples.len() as f64 / ratio) as usize;
    let mut out = Vec::with_capacity(out_len);
    for i in 0..out_len {
        let pos = i as f64 * ratio;
        let idx = pos as usize;
        let frac = (pos - idx as f64) as f32;
        let a = samples[idx];
        let b = samples.get(idx + 1).copied().unwrap_or(a);
        let s = a + (b - a) * frac;
        out.push((s.clamp(-1.0, 1.0) * 32767.0) as i16);
    }
    out
}

/// Encode 16 kHz mono 16-bit PCM as a WAV in memory. The header is written
/// by hand because the input is fixed-format (hound can't hand back the
/// buffer, and its Seek dance buys nothing for one layout).
fn write_wav(samples: &[i16]) -> Result<Vec<u8>> {
    let data_size = (samples.len() * 2) as u32;
    let mut out = Vec::with_capacity(44 + samples.len() * 2);
    out.extend_from_slice(b"RIFF");
    push_u32(&mut out, 36 + data_size);
    out.extend_from_slice(b"WAVE");
    out.extend_from_slice(b"fmt ");
    push_u32(&mut out, 16); // PCM chunk size
    push_u16(&mut out, 1); // PCM format
    push_u16(&mut out, 1); // mono
    push_u32(&mut out, TARGET_SAMPLE_RATE);
    push_u32(&mut out, TARGET_SAMPLE_RATE * 2); // byte rate
    push_u16(&mut out, 2); // block align
    push_u16(&mut out, 16); // bits per sample
    out.extend_from_slice(b"data");
    push_u32(&mut out, data_size);
    for &s in samples {
        out.extend_from_slice(&s.to_le_bytes());
    }
    Ok(out)
}

fn push_u16(out: &mut Vec<u8>, v: u16) {
    out.extend_from_slice(&v.to_le_bytes());
}

fn push_u32(out: &mut Vec<u8>, v: u32) {
    out.extend_from_slice(&v.to_le_bytes());
}
