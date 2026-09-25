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
    if (getSinkFormatSupport(format) == AudioSink.SINK_FORMAT_SUPPORTED_DIRECTLY) {
      return C.FORMAT_UNSUPPORTED_SUBTYPE
    }
    val rate = if (format.sampleRate == Format.NO_VALUE) 48_000 else format.sampleRate
    if (!sinkSupportsFormat(Util.getPcmFormat(C.ENCODING_PCM_16BIT, 2, rate))) {
      return C.FORMAT_UNSUPPORTED_SUBTYPE
    }
    return C.FORMAT_HANDLED
  }

  override fun createDecoder(format: Format, cryptoConfig: CryptoConfig?): Ac3goDecoder {
    val inputSize = if (format.maxInputSize != Format.NO_VALUE) format.maxInputSize else DEFAULT_INPUT_SIZE
    val maxChannels = outputMaxChannels()
    Log.i(TAG, "ac3go decoding ${format.sampleMimeType} ${format.channelCount}ch ${format.sampleRate}Hz, output up to ${maxChannels}ch")
    return Ac3goDecoder(BUFFERS, BUFFERS, inputSize, maxChannels)
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

/** One ac3go FrameDecoder behind media3's decoder thread. */
// SimpleDecoder fills the arrays itself (createInputBuffer/createOutputBuffer).
@Suppress("UNCHECKED_CAST")
@UnstableApi
class Ac3goDecoder(inputs: Int, outputs: Int, inputSize: Int, maxChannels: Int) :
  SimpleDecoder<DecoderInputBuffer, SimpleDecoderOutputBuffer, Ac3goDecoderException>(
    arrayOfNulls<DecoderInputBuffer>(inputs) as Array<DecoderInputBuffer>,
    arrayOfNulls<SimpleDecoderOutputBuffer>(outputs) as Array<SimpleDecoderOutputBuffer>,
  ) {

  private val go: FrameDecoder = Offlineaudio.newFrameDecoder(maxChannels.toLong())

  @Volatile var sampleRate: Int = 48_000
    private set

  @Volatile var channels: Int = 2
    private set

  init {
    setInitialInputBufferSize(inputSize)
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
    if (pcm == null || pcm.isEmpty()) {
      outputBuffer.shouldBeSkipped = true
      return null
    }
    go.sampleRate().toInt().let { if (it > 0) sampleRate = it }
    go.channels().toInt().let { if (it > 0) channels = it }
    outputBuffer.init(inputBuffer.timeUs, pcm.size).put(pcm).flip()
    return null
  }
}
