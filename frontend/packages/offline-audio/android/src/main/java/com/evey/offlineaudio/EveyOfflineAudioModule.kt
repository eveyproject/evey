package com.evey.offlineaudio

import android.content.Context
import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import android.os.PowerManager
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

    OnCreate { appContext.reactContext?.let { Ac3goGuard.init(it) } }

    // Codecs the player decodes itself through Ac3goAudioRenderer, declared
    // to the server whatever the device's own decoders or audio output say -
    // unless Ac3goGuard has a reason to doubt ac3go on this device.
    Function("playbackDecoders") { Ac3goGuard.playbackDecoders() }

    // The launch check: the benchmark (once per process), the verdict, and a
    // previous run that died while ac3go was decoding.
    AsyncFunction("checkPlaybackDecoder") {
      val context = appContext.reactContext ?: throw IllegalStateException("no react context")
      val bench = Ac3goGuard.benchOnce(context)
      mapOf(
        "decoders" to Ac3goGuard.playbackDecoders(),
        "blockedBy" to (Ac3goGuard.blockedBy() ?: ""),
        "bench" to bench,
        "abnormalExit" to Ac3goGuard.abnormalExit(),
      )
    }

    // The player's watchdog saw ac3go fall behind: not declared any more.
    Function("disablePlaybackDecoder") { reason: String -> Ac3goGuard.disable(reason) }

    // The app left the screen: a kill from now on is not ac3go's doing.
    Function("markPlaybackDecoderIdle") { Ac3goGuard.markStopped() }

    Function("setPlaybackDecoderMedia") { mediaId: String -> Ac3goGuard.setMedia(mediaId) }

    // What Ac3goAudioRenderer's decoder has handed the sink in this playback
    // (or the last one), null when ac3go never took a track.
    Function("playbackDecoderStats") { Ac3goPlaybackStats.snapshot() }

    Function("resetPlaybackDecoderStats") { Ac3goPlaybackStats.reset() }

    Function("setNotificationTitle") { title: String ->
      if (title.isNotBlank()) OfflineAudioService.title = title
    }

    AsyncFunction("optimize") { inPath: String, outPath: String, label: String, promise: Promise ->
      if (!running.compareAndSet(false, true)) {
        promise.reject(CodedException("busy", "an optimization pass is already running", null))
        return@AsyncFunction
      }
      cancelled = false
      thread(name = "evey-offline-audio") {
        try {
          val summary = runPass(inPath, outPath, label)
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

    // Reassemble a size-reduced (720p) download from the slices the server
    // handed over. Same module because it is the same embedded mkvgo, and the
    // same one-heavy-job-at-a-time rule: an audio pass and a join both hammer
    // flash I/O, and two at once would thrash it.
    AsyncFunction("joinSlices") { dir: String, count: Int, outPath: String, durationMs: Long, label: String, promise: Promise ->
      if (!running.compareAndSet(false, true)) {
        promise.reject(CodedException("busy", "another offline job is already running", null))
        return@AsyncFunction
      }
      thread(name = "evey-offline-join") {
        try {
          promise.resolve(runJoin(dir, count, outPath, durationMs, label))
        } catch (t: Throwable) {
          val msg = t.message ?: "unknown"
          promise.reject(CodedException(msg.substringBefore(":").ifEmpty { "failed" }, msg, t))
        } finally {
          running.set(false)
        }
      }
    }

    // Drop the collected slices. Deliberately NOT folded into joinSlices: the
    // slices are the only thing between a crash and re-downloading gigabytes,
    // so JS deletes them once it has recorded the finished file.
    Function("deleteSlices") { dir: String, count: Int ->
      Offlineaudio.deleteSlices(dir, count.toLong())
    }

    Function("cancel") {
      cancelled = true
      Offlineaudio.cancelJoin()
    }

    Function("isRunning") {
      running.get()
    }
  }

  /**
   * Assemble a size-reduced download from its slices. Minutes of flash I/O on
   * a multi-gigabyte file, so it gets the same foreground service + wakelock
   * treatment as the audio pass: without them the OEM freezer suspends the
   * process the moment the app leaves the screen and the join stops mid-file.
   */
  private fun runJoin(dir: String, count: Int, outPath: String, durationMs: Long, label: String): Map<String, Any> {
    val context = appContext.reactContext ?: throw IllegalStateException("no react context")

    var serviceStarted = false
    try {
      OfflineAudioService.start(context, label)
      serviceStarted = true
    } catch (t: Throwable) {
      logW(NAME_TAG, "foreground service unavailable: ${t.message}")
    }
    val wakeLock = (context.getSystemService(Context.POWER_SERVICE) as PowerManager)
      .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "evey:offline-join")
    runCatching { wakeLock.acquire(WAKELOCK_TIMEOUT_MS) }

    // A copy this long behind a label that never moves reads as a hang - it
    // did, in the field, on the server-side preparation. Poll and report.
    val done = AtomicBoolean(false)
    val poller = thread(name = "evey-offline-join-progress") {
      while (!done.get()) {
        val permille = Offlineaudio.joinPermille()
        sendEvent("progress", mapOf("stage" to "join", "permille" to permille))
        if (serviceStarted) OfflineAudioService.updateProgress(context, label, permille)
        Thread.sleep(PROGRESS_INTERVAL_MS)
      }
    }
    try {
      Offlineaudio.joinSlices(dir, count.toLong(), outPath, durationMs)
    } finally {
      done.set(true)
      poller.join()
      runCatching { if (wakeLock.isHeld) wakeLock.release() }
      if (serviceStarted) runCatching { OfflineAudioService.stop(context) }
    }
    sendEvent("progress", mapOf("stage" to "join", "permille" to 1000L))
    return mapOf("slices" to count, "path" to outPath)
  }

  private fun runPass(inPath: String, outPath: String, label: String): Map<String, Any> {
    val context = appContext.reactContext
      ?: throw IllegalStateException("no react context")
    val cacheDir = context.cacheDir.absolutePath

    // Foreground service + partial wakelock for the pass's lifetime: without
    // them the OEM freezer suspends the process as soon as the app leaves the
    // screen and the pass silently stops mid-file. Best-effort - Android 12+
    // may refuse a foreground start from the background, and the pass must
    // still run (unprotected) rather than fail.
    var serviceStarted = false
    try {
      OfflineAudioService.start(context, label)
      serviceStarted = true
    } catch (t: Throwable) {
      logW(NAME_TAG, "foreground service unavailable: ${t.message}")
    }
    val wakeLock = (context.getSystemService(Context.POWER_SERVICE) as PowerManager)
      .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "evey:offline-audio")
    runCatching { wakeLock.acquire(WAKELOCK_TIMEOUT_MS) }

    val session = Offlineaudio.open(inPath, cacheDir)
    try {
      encode(session) { permille ->
        if (serviceStarted) OfflineAudioService.updateProgress(context, label, permille)
      }
      throwIfCancelled()
      remuxWithProgress(session, outPath) { permille ->
        if (serviceStarted) OfflineAudioService.updateProgress(context, label, permille)
      }
      return mapOf(
        "sampleRate" to session.sampleRate().toInt(),
        "sourceCodec" to session.sourceCodec(),
        "sourceChannels" to session.sourceChannels().toInt(),
        "aacFrames" to session.aacFrames(),
        "skippedFrames" to session.skippedFrames(),
      )
    } finally {
      session.close()
      runCatching { if (wakeLock.isHeld) wakeLock.release() }
      if (serviceStarted) runCatching { OfflineAudioService.stop(context) }
    }
  }

  /** Pump: Go PCM -> MediaCodec -> AAC frames back into Go's intermediate. */
  private fun encode(session: Session, onNotifyProgress: (Long) -> Unit) {
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

    // The pump must SATURATE the encoder: fill every free input buffer, drain
    // every ready output buffer, and only block when neither side moved. The
    // first field run paced itself instead - one input, one output and up to
    // 20 ms of blocking timeouts per iteration is one ~21 ms audio frame per
    // ~30 ms of wall clock, barely faster than just playing the film.
    var outputDone = false
    val startedAt = System.currentTimeMillis()

    fun feedOne(): Boolean {
      val inIdx = codec.dequeueInputBuffer(0)
      if (inIdx < 0) return false
      if (pcm == null || pcmOff >= pcm!!.size) {
        pcm = session.readPCM()
        pcmOff = 0
      }
      val chunk = pcm
      if (chunk == null || chunk.isEmpty()) {
        codec.queueInputBuffer(inIdx, 0, 0, 0, MediaCodec.BUFFER_FLAG_END_OF_STREAM)
        inputDone = true
        return true
      }
      val buf = codec.getInputBuffer(inIdx)!!
      val n = minOf(buf.remaining(), chunk.size - pcmOff)
      buf.put(chunk, pcmOff, n)
      pcmOff += n
      // MediaCodec wants monotonic timestamps; derived from the sample count,
      // they are exact. The Go writer re-derives its own from the frame
      // count, so these never reach the output file.
      val ptsUs = samplesIn * 1_000_000L / sampleRate
      samplesIn += n / BYTES_PER_FRAME
      codec.queueInputBuffer(inIdx, 0, n, ptsUs, 0)
      return true
    }

    fun drainOne(timeoutUs: Long): Boolean {
      val outIdx = codec.dequeueOutputBuffer(info, timeoutUs)
      when {
        outIdx == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED -> {
          if (!began) {
            val csd = codec.outputFormat.getByteBuffer("csd-0")
              ?: throw IllegalStateException("encoder exposed no csd-0")
            val asc = ByteArray(csd.remaining()).also { csd.get(it) }
            session.beginAAC(asc)
            began = true
          }
          return true
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
          if (info.flags and MediaCodec.BUFFER_FLAG_END_OF_STREAM != 0) outputDone = true
          return true
        }
        else -> return false
      }
    }

    try {
      while (!outputDone) {
        throwIfCancelled()
        var progressed = false
        while (!inputDone && feedOne()) progressed = true
        while (!outputDone && drainOne(0)) progressed = true
        if (!progressed && !outputDone) {
          // Nothing to feed, nothing ready: yield until the encoder moves.
          drainOne(DEQUEUE_TIMEOUT_US)
        }

        val now = System.currentTimeMillis()
        if (now - lastEvent >= PROGRESS_INTERVAL_MS) {
          lastEvent = now
          val permille = session.decodeProgressPermille()
          sendEvent("progress", mapOf("stage" to "transcode", "permille" to permille))
          onNotifyProgress(permille * 8 / 10)
          // Remote-diagnosable throughput: permille over elapsed is the whole
          // story of "is it slow", and logcat is the only channel a field
          // device reliably offers.
          android.util.Log.i(NAME_TAG, "transcode permille=$permille elapsedMs=${now - startedAt}")
        }
      }
    } finally {
      runCatching { codec.stop() }
      codec.release()
    }
    session.finishAAC()
  }

  /** Remux blocks for minutes; a sidecar thread turns its permille into events. */
  private fun remuxWithProgress(session: Session, outPath: String, onNotifyProgress: (Long) -> Unit) {
    val done = AtomicBoolean(false)
    val poller = thread(name = "evey-offline-audio-progress") {
      while (!done.get()) {
        val permille = session.remuxPermille()
        sendEvent("progress", mapOf("stage" to "remux", "permille" to permille))
        onNotifyProgress(800 + permille * 2 / 10)
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

  private fun logW(tag: String, msg: String) {
    android.util.Log.w(tag, msg)
  }

  private companion object {
    const val AAC_BITRATE = 160_000
    const val INPUT_CHUNK = 64 * 1024
    const val BYTES_PER_FRAME = 4 // stereo s16le
    const val DEQUEUE_TIMEOUT_US = 10_000L
    const val PROGRESS_INTERVAL_MS = 500L
    const val NAME_TAG = "EveyOfflineAudio"
    // Safety valve, not a design bound: a pass that still runs after an hour
    // is stuck, and a stuck pass must not hold the CPU awake forever.
    const val WAKELOCK_TIMEOUT_MS = 60L * 60 * 1000
  }
}
