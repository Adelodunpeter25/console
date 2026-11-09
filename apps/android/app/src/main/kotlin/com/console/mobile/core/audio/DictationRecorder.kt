package com.console.mobile.core.audio

import android.content.Context
import android.media.MediaRecorder
import java.io.File

/**
 * Microphone capture for speech-to-text dictation. Records to a cache file
 * in memory-safe AAC/M4A (16 kHz mono — the rate Whisper likes), reads the
 * bytes out on stop, and deletes the file. Nothing touches persistent
 * storage; the bytes live only for the transcribe request.
 */

/** A longer take would exceed the server's 4 MB audio cap, so stop here. */
const val MAX_RECORD_SECONDS = 120

class DictationRecorder(private val context: Context) {
    private var recorder: MediaRecorder? = null
    private var outputFile: File? = null

    fun start() {
        val file = File(context.cacheDir, "dictation-${System.currentTimeMillis()}.m4a")
        val next = MediaRecorder().apply {
            setAudioSource(MediaRecorder.AudioSource.MIC)
            setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
            setAudioSamplingRate(16_000)
            setAudioChannels(1)
            setAudioEncodingBitRate(32_000)
            setOutputFile(file)
        }
        next.prepare()
        next.start()
        outputFile?.delete()
        recorder = next
        outputFile = file
    }

    /** Stop and return the take's bytes. Deletes the cache file. */
    fun stop(): ByteArray {
        val active = recorder ?: error("Not recording")
        recorder = null
        var bytes = ByteArray(0)
        try {
            active.stop()
            bytes = outputFile?.takeIf { it.exists() }?.readBytes() ?: ByteArray(0)
        } finally {
            active.release()
            outputFile?.delete()
            outputFile = null
        }
        return bytes
    }

    /** Abandon the take without transcribing. */
    fun cancel() {
        recorder?.release()
        recorder = null
        outputFile?.delete()
        outputFile = null
    }
}
