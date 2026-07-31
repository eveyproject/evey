package com.evey.offlineaudio

import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import expo.modules.kotlin.Promise
import expo.modules.kotlin.exception.CodedException
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import java.io.File
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread

/**
 * Drives the offline audio compatibility pass: the embedded Go pipeline
 * (ac3go decode with bitstream-coefficient stereo downmix, mkvgo demux/remux)
 * on one side, Android's hardware AAC encoder on the other, this module being
 * the pump between the two.
 *
 * Why the encoder lives here and not in Go: MediaCodec is the one encoder
 * every device ships with hardware support for, and AAC is the one codec
 * ExoPlayer renders everywhere. The Go side owns everything container- and
 * bitstream-shaped; this side owns exactly one thing, PCM in / AAC out.
 *
 * One pass at a time: the pass is minutes of sustained CPU and tens of GB of
 * flash I/O, and two at once would thrash both. Concurrent optimize() calls
 * are refused, not queued - the queue lives on the JS side where it can
 * persist across app restarts.
 */
class EveyOfflineAudioModule : Module() {

  private val running = AtomicBoolean(false)
  @Volatile private var cancelled = false

  override fun definition() = ModuleDefinition {
    Name("EveyOfflineAudio")

    Events("progress")

    AsyncFunction("optimize") { inPath: String, outPath: String, promise: Promise ->
      if (!running.compareAndSet(false, true)) {
        promise.reject(CodedException("busy", "an optimization pass is already running", null))
        return@AsyncFunction
      }
      cancelled = false
      thread(name = "evey-offline-audio") {
        try {
          val summary = runPass(inPath, outPath)
          promise.resolve(summary)
        } catch (t: Throwable) {
          // The Go error message's prefix is the machine-readable part
          // ("unsupported-codec:dts", "not-matroska", ...); pass it through
          // as the code so JS routes without string-scraping the message.
          File(outPath).delete()
          val msg = t.message ?: "unknown"
          promise.reject(CodedException(msg.substringBefore(":").ifEmpty { "failed" }, msg, t))
        } finally {
          running.set(false)
        }
      }
    }

    Function("cancel") {
      cancelled = true
    }

    Function("isRunning") {
      running.get()
    }
  }

  private fun runPass(inPath: String, outPath: String): Map<String, Any> {
    val cacheDir = appContext.reactContext?.cacheDir?.absolutePath
      ?: throw IllegalStateException("no react context")
    val session = Offlineaudio.open(inPath, cacheDir)
    try {
      encode(session)
      throwIfCancelled()
      remuxWithProgress(session, outPath)
      return mapOf(
        "sampleRate" to session.sampleRate().toInt(),
        "sourceCodec" to session.sourceCodec(),
        "sourceChannels" to session.sourceChannels().toInt(),
        "aacFrames" to session.aacFrames(),
        "skippedFrames" to session.skippedFrames(),
      )
    } finally {
      session.close()
    }
  }

  /** Pump: Go PCM -> MediaCodec -> AAC frames back into Go's intermediate. */
  private fun encode(session: Session) {
    val sampleRate = session.sampleRate().toInt()
    val format = MediaFormat.createAudioFormat(MediaFormat.MIMETYPE_AUDIO_AAC, sampleRate, 2).apply {
      setInteger(MediaFormat.KEY_AAC_PROFILE, MediaCodecInfo.CodecProfileLevel.AACObjectLC)
      setInteger(MediaFormat.KEY_BIT_RATE, AAC_BITRATE)
      setInteger(MediaFormat.KEY_MAX_INPUT_SIZE, INPUT_CHUNK)
    }
    val codec = MediaCodec.createEncoderByType(MediaFormat.MIMETYPE_AUDIO_AAC)
    codec.configure(format, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
    codec.start()

    var began = false
    var pcm: ByteArray? = null
    var pcmOff = 0
    var inputDone = false
    var samplesIn = 0L
    var lastEvent = 0L
    val info = MediaCodec.BufferInfo()

    try {
      while (true) {
        throwIfCancelled()

        if (!inputDone) {
          val inIdx = codec.dequeueInputBuffer(DEQUEUE_TIMEOUT_US)
          if (inIdx >= 0) {
            if (pcm == null || pcmOff >= pcm.size) {
              pcm = session.readPCM()
              pcmOff = 0
            }
            val chunk = pcm
            if (chunk == null || chunk.isEmpty()) {
              codec.queueInputBuffer(inIdx, 0, 0, 0, MediaCodec.BUFFER_FLAG_END_OF_STREAM)
              inputDone = true
            } else {
              val buf = codec.getInputBuffer(inIdx)!!
              val n = minOf(buf.remaining(), chunk.size - pcmOff)
              buf.put(chunk, pcmOff, n)
              pcmOff += n
              // MediaCodec wants monotonic timestamps; derived from the sample
              // count, they are exact. The Go writer re-derives its own from
              // the frame count, so these never reach the output file.
              val ptsUs = samplesIn * 1_000_000L / sampleRate
              samplesIn += n / BYTES_PER_FRAME
              codec.queueInputBuffer(inIdx, 0, n, ptsUs, 0)
            }
          }
        }

        val outIdx = codec.dequeueOutputBuffer(info, DEQUEUE_TIMEOUT_US)
        when {
          outIdx == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED -> {
            if (!began) {
              val csd = codec.outputFormat.getByteBuffer("csd-0")
                ?: throw IllegalStateException("encoder exposed no csd-0")
              val asc = ByteArray(csd.remaining()).also { csd.get(it) }
              session.beginAAC(asc)
              began = true
            }
          }
          outIdx >= 0 -> {
            val isConfig = info.flags and MediaCodec.BUFFER_FLAG_CODEC_CONFIG != 0
            if (!isConfig && info.size > 0) {
              if (!began) throw IllegalStateException("AAC frame before csd-0")
              val frame = ByteArray(info.size)
              codec.getOutputBuffer(outIdx)!!.also { it.position(info.offset) }.get(frame)
              session.writeAAC(frame)
            }
            codec.releaseOutputBuffer(outIdx, false)
            if (info.flags and MediaCodec.BUFFER_FLAG_END_OF_STREAM != 0) break
          }
        }

        val now = System.currentTimeMillis()
        if (now - lastEvent >= PROGRESS_INTERVAL_MS) {
          lastEvent = now
          sendEvent("progress", mapOf("stage" to "transcode", "permille" to session.decodeProgressPermille()))
        }
      }
    } finally {
      runCatching { codec.stop() }
      codec.release()
    }
    session.finishAAC()
  }

  /** Remux blocks for minutes; a sidecar thread turns its permille into events. */
  private fun remuxWithProgress(session: Session, outPath: String) {
    val done = AtomicBoolean(false)
    val poller = thread(name = "evey-offline-audio-progress") {
      while (!done.get()) {
        sendEvent("progress", mapOf("stage" to "remux", "permille" to session.remuxPermille()))
        Thread.sleep(PROGRESS_INTERVAL_MS)
      }
    }
    try {
      session.remux(outPath)
    } finally {
      done.set(true)
      poller.join()
    }
    sendEvent("progress", mapOf("stage" to "remux", "permille" to 1000L))
  }

  private fun throwIfCancelled() {
    if (cancelled) throw CodedException("cancelled", "optimization cancelled", null)
  }

  private companion object {
    const val AAC_BITRATE = 160_000
    const val INPUT_CHUNK = 64 * 1024
    const val BYTES_PER_FRAME = 4 // stereo s16le
    const val DEQUEUE_TIMEOUT_US = 10_000L
    const val PROGRESS_INTERVAL_MS = 500L
  }
}
