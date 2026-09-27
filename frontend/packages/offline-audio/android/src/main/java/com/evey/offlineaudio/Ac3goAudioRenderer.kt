package com.evey.offlineaudio

import android.content.Context
import android.content.IntentFilter
import android.media.AudioManager
import android.os.Handler
import android.util.Log
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.UnstableApi
import androidx.media3.common.util.Util
import androidx.media3.decoder.CryptoConfig
import androidx.media3.decoder.DecoderException
import androidx.media3.decoder.DecoderInputBuffer
import androidx.media3.decoder.SimpleDecoder
import androidx.media3.decoder.SimpleDecoderOutputBuffer
import androidx.media3.exoplayer.audio.AudioRendererEventListener
import androidx.media3.exoplayer.audio.AudioSink
import androidx.media3.exoplayer.audio.DecoderAudioRenderer

/**
 * Plays AC-3/E-AC-3 through ac3go ahead of the device's own decoders: one
 * behaviour on every phone, including the ones whose Dolby decoder exists but
 * renders choppy, and on every TV box whose output takes no Dolby bitstream.
 * An output that does take the bitstream keeps it: this renderer steps aside
 * and MediaCodecAudioRenderer passes it through to the receiver. expo-video's
 * patched renderers factory builds it by reflection (class name + this
 * constructor are the contract).
 */
@UnstableApi
class Ac3goAudioRenderer(
  private val context: Context,
  handler: Handler?,
  listener: AudioRendererEventListener?,
  sink: AudioSink,
) : DecoderAudioRenderer<Ac3goDecoder>(handler, listener, sink) {

  override fun getName() = "Ac3goAudioRenderer"

  override fun supportsFormatInternal(format: Format): Int {
    if (format.sampleMimeType !in MIME_TYPES) return C.FORMAT_UNSUPPORTED_TYPE
    val bitstream = getSinkFormatSupport(format)
    val rate = if (format.sampleRate == Format.NO_VALUE) 48_000 else format.sampleRate
    val pcm = sinkSupportsFormat(Util.getPcmFormat(C.ENCODING_PCM_16BIT, 2, rate))
    val support = when {
      bitstream == AudioSink.SINK_FORMAT_SUPPORTED_DIRECTLY -> C.FORMAT_UNSUPPORTED_SUBTYPE
      !pcm -> C.FORMAT_UNSUPPORTED_SUBTYPE
      else -> C.FORMAT_HANDLED
    }
    Log.i(TAG, "${format.sampleMimeType}: sink bitstream=$bitstream pcm=$pcm -> ${if (support == C.FORMAT_HANDLED) "ac3go" else "device"}")
    return support
  }

  override fun createDecoder(format: Format, cryptoConfig: CryptoConfig?): Ac3goDecoder {
    val inputSize = if (format.maxInputSize != Format.NO_VALUE) format.maxInputSize else DEFAULT_INPUT_SIZE
    val maxChannels = outputMaxChannels()
    Log.i(TAG, "ac3go decoding ${format.sampleMimeType} ${format.channelCount}ch ${format.sampleRate}Hz, output up to ${maxChannels}ch")
    val codec = if (format.sampleMimeType == MimeTypes.AUDIO_AC3) "ac3" else "eac3"
    return Ac3goDecoder(BUFFERS, BUFFERS, inputSize, maxChannels, codec, format.channelCount)
  }

  override fun getOutputFormat(decoder: Ac3goDecoder): Format =
    Util.getPcmFormat(C.ENCODING_PCM_16BIT, decoder.channels, decoder.sampleRate)

  // A plugged HDMI sink states its channel count (the same evidence the audio
  // probe reports); any other output, or none stated, gets ac3go's stereo.
  private fun outputMaxChannels(): Int {
    val hdmi = runCatching {
      context.registerReceiver(null, IntentFilter(AudioManager.ACTION_HDMI_AUDIO_PLUG))
    }.getOrNull() ?: return 2
    if (hdmi.getIntExtra(AudioManager.EXTRA_AUDIO_PLUG_STATE, 0) != 1) return 2
    return hdmi.getIntExtra(AudioManager.EXTRA_MAX_CHANNEL_COUNT, 2).coerceIn(2, 8)
  }

  companion object {
    private const val TAG = "EveyAc3go"
    val MIME_TYPES = setOf(MimeTypes.AUDIO_AC3, MimeTypes.AUDIO_E_AC3, MimeTypes.AUDIO_E_AC3_JOC)
    private const val BUFFERS = 16
    // An E-AC-3 MP4 sample can hold a few syncframes of up to 4 KiB each.
    private const val DEFAULT_INPUT_SIZE = 16 * 1024
  }
}

class Ac3goDecoderException(message: String, cause: Throwable?) : DecoderException(message, cause)

/**
 * The decoder of the current playback, for the JS telemetry poll. The last
 * snapshot outlives the decoder so a report sent as the player closes still
 * has the whole run.
 */
@androidx.annotation.OptIn(markerClass = [UnstableApi::class])
object Ac3goPlaybackStats {
  @Volatile private var current: Ac3goDecoder? = null
  @Volatile private var last: Map<String, Any>? = null

  fun attach(decoder: Ac3goDecoder) {
    current = decoder
    last = null
  }

  fun detach(decoder: Ac3goDecoder) {
    if (current !== decoder) return
    last = decoder.snapshot()
    current = null
  }

  fun snapshot(): Map<String, Any>? = current?.snapshot() ?: last

  /**
   * A new playback starts: forget the previous one, including a decoder not
   * released yet, so a title ac3go never decodes reports nothing rather than
   * the counters of the one before.
   */
  fun reset() {
    current = null
    last = null
  }
}

/** One ac3go FrameDecoder behind media3's decoder thread. */
// SimpleDecoder fills the arrays itself (createInputBuffer/createOutputBuffer).
@Suppress("UNCHECKED_CAST")
@UnstableApi
class Ac3goDecoder(
  inputs: Int,
  outputs: Int,
  inputSize: Int,
  private val maxChannels: Int,
  private val codec: String,
  private val sourceChannels: Int,
) :
  SimpleDecoder<DecoderInputBuffer, SimpleDecoderOutputBuffer, Ac3goDecoderException>(
    arrayOfNulls<DecoderInputBuffer>(inputs) as Array<DecoderInputBuffer>,
    arrayOfNulls<SimpleDecoderOutputBuffer>(outputs) as Array<SimpleDecoderOutputBuffer>,
  ) {

  private val go: FrameDecoder = Offlineaudio.newFrameDecoder(maxChannels.toLong())

  @Volatile var sampleRate: Int = 48_000
    private set

  @Volatile var channels: Int = 2
    private set

  // Copied from Go on the decoder thread after each unit: the FrameDecoder is
  // not safe to read from the JS thread that polls these.
  @Volatile private var frames = 0L
  @Volatile private var badFrames = 0L
  @Volatile private var silentFrames = 0L
  @Volatile private var silentRunFrames = 0L
  // ac3go's own counters as one line, refreshed every SUMMARY_EVERY frames:
  // a string per access unit would be garbage at 31 units a second.
  @Volatile private var summary = ""
  private var summaryAt = -SUMMARY_EVERY

  init {
    setInitialInputBufferSize(inputSize)
    Ac3goPlaybackStats.attach(this)
  }

  /** What the player heard so far, the shape the telemetry beacon reports. */
  fun snapshot(): Map<String, Any> = mapOf(
    "codec" to codec,
    "sourceChannels" to sourceChannels,
    "outputChannels" to channels,
    "maxChannels" to maxChannels,
    "sampleRate" to sampleRate,
    "frames" to frames,
    "badFrames" to badFrames,
    "silentFrames" to silentFrames,
    "silentRunFrames" to silentRunFrames,
    "decoder" to summary,
  )

  override fun release() {
    super.release()
    summary = go.decoderSummary()
    Log.i("EveyAc3go", "ac3go released ${snapshot()}")
    Ac3goPlaybackStats.detach(this)
  }

  override fun getName() = "ac3go"

  override fun createInputBuffer() = DecoderInputBuffer(DecoderInputBuffer.BUFFER_REPLACEMENT_MODE_DIRECT)

  override fun createOutputBuffer() = SimpleDecoderOutputBuffer { releaseOutputBuffer(it) }

  override fun createUnexpectedDecodeException(error: Throwable) =
    Ac3goDecoderException("unexpected ac3go decode error", error)

  override fun decode(
    inputBuffer: DecoderInputBuffer,
    outputBuffer: SimpleDecoderOutputBuffer,
    reset: Boolean,
  ): Ac3goDecoderException? {
    if (reset) go.reset()
    val data = inputBuffer.data ?: return null
    val au = ByteArray(data.remaining())
    data.get(au)
    val pcm = try {
      go.decode(au)
    } catch (e: Exception) {
      return Ac3goDecoderException("ac3go decode failed", e)
    }
    frames = go.frames()
    badFrames = go.badFrames()
    silentFrames = go.silentFrames()
    silentRunFrames = go.silentRunFrames()
    if (frames - summaryAt >= SUMMARY_EVERY) {
      summary = go.decoderSummary()
      summaryAt = frames
    }
    if (pcm == null || pcm.isEmpty()) {
      outputBuffer.shouldBeSkipped = true
      return null
    }
    go.sampleRate().toInt().let { if (it > 0) sampleRate = it }
    go.channels().toInt().let { if (it > 0) channels = it }
    outputBuffer.init(inputBuffer.timeUs, pcm.size).put(pcm).flip()
    return null
  }

  private companion object {
    const val SUMMARY_EVERY = 64L
  }
}
