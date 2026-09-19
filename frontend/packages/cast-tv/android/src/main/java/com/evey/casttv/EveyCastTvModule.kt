package com.evey.casttv

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.support.v4.media.MediaMetadataCompat
import android.support.v4.media.session.MediaSessionCompat
import android.support.v4.media.session.PlaybackStateCompat
import com.google.android.gms.cast.MediaLoadRequestData
import com.google.android.gms.cast.tv.CastReceiverContext
import com.google.android.gms.cast.tv.media.MediaManager
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import org.json.JSONObject

/**
 * Cast Connect for the Evey TV app.
 *
 * The division of labour, which is the whole design:
 *
 *   - the EVEY PLAYER remains the source of truth for playback. It decides
 *     what plays, at what position, with which tracks, exactly as it does
 *     when someone presses play on the remote. Nothing here holds a second
 *     copy of that state.
 *   - this MediaSession is its PROJECTION. JS pushes the player's state into
 *     it (`setPlaybackState`), and Android turns that into what the remote's
 *     media keys act on and what the Cast SDK republishes to every connected
 *     sender as MediaStatus. That is why a phone or the web modal stays in
 *     sync without a line of code of its own.
 *   - commands arrive the same way, in reverse: the remote or a sender acts
 *     on the MediaSession, we forward the intent to JS as an event, and the
 *     Evey player applies it. The session never moves the playhead itself -
 *     a projection that starts deciding is a second state machine, and two
 *     state machines over one film is how playback bugs are born.
 *
 * A LOAD carries a media id (see castLaunch.ts), never a URL: the app plays
 * it through its own pipeline, with its own capabilities. The URL in the
 * request belongs to the Web Receiver, which is what plays this cast on every
 * device that is not this app.
 */
class EveyCastTvModule : Module() {

  private var session: MediaSessionCompat? = null
  private var mediaManager: MediaManager? = null
  private var started = false
  // A cold launch delivers the LOAD before React Native has finished booting,
  // so there is no JS listener yet. The payload waits here and is replayed
  // the moment JS subscribes - without this, casting to a closed app opens
  // the home screen and nothing plays.
  private var pendingLoad: Bundle? = null
  private var jsListening = false

  private val context: Context
    get() = requireNotNull(appContext.reactContext) { "no react context" }

  override fun definition() = ModuleDefinition {
    Name("EveyCastTv")

    Events("onCastLoad", "onCastCommand")

    // Starts the receiver: from here the app is a Cast Connect target. Safe
    // to call twice (the TV app calls it on every mount).
    Function("start") {
      if (started) return@Function true
      // initInstance before the first getInstance(). Google's sample puts it
      // in Application.onCreate; doing it here keeps the whole feature inside
      // this module instead of patching a MainApplication that `prebuild`
      // regenerates (apps/tv/android is not versioned).
      CastReceiverContext.initInstance(context.applicationContext)
      val receiver = CastReceiverContext.getInstance()
      session = MediaSessionCompat(context, "EveyTV").also { s ->
        s.setCallback(SessionCallback())
        s.isActive = true
        receiver.mediaManager.setSessionCompatToken(s.sessionToken)
      }
      mediaManager = receiver.mediaManager.also { it.setMediaLoadCommandCallback(LoadCallback()) }
      receiver.start()
      started = true
      // A cold launch IS an intent: the platform started the activity with
      // the cast request already in it, and it has therefore been delivered
      // before this module existed. Hand it over now, or a cast to a closed
      // app opens the home screen and plays nothing.
      appContext.currentActivity?.intent?.let { mediaManager?.onNewIntent(it) }
      true
    }

    Function("stop") {
      if (!started) return@Function true
      try {
        CastReceiverContext.getInstance().stop()
      } catch (_: Throwable) {
        // Stopping a receiver that never started must never crash the app.
      }
      session?.isActive = false
      session?.release()
      session = null
      mediaManager = null
      started = false
      true
    }

    // JS declares itself ready; any LOAD that arrived during the cold boot is
    // replayed now.
    Function("acknowledgeReady") {
      jsListening = true
      pendingLoad?.let { queued ->
        pendingLoad = null
        sendEvent("onCastLoad", queued)
      }
      true
    }

    // The player's state, pushed on every meaningful change. This is a
    // projection: we write what JS says, we never compute it.
    Function("setPlaybackState") { positionMs: Double, durationMs: Double, playing: Boolean, speed: Double ->
      val s = session ?: return@Function false
      val actions = PlaybackStateCompat.ACTION_PLAY or
        PlaybackStateCompat.ACTION_PAUSE or
        PlaybackStateCompat.ACTION_PLAY_PAUSE or
        PlaybackStateCompat.ACTION_SEEK_TO or
        PlaybackStateCompat.ACTION_STOP or
        PlaybackStateCompat.ACTION_SKIP_TO_NEXT or
        PlaybackStateCompat.ACTION_SKIP_TO_PREVIOUS
      s.setPlaybackState(
        PlaybackStateCompat.Builder()
          .setActions(actions)
          .setState(
            if (playing) PlaybackStateCompat.STATE_PLAYING else PlaybackStateCompat.STATE_PAUSED,
            positionMs.toLong(),
            // A paused projection must advertise rate 0, or the sender's
            // scrubber keeps extrapolating forward over a frozen picture.
            if (playing) speed.toFloat() else 0f,
          )
          .build(),
      )
      if (durationMs > 0) {
        s.setMetadata(
          MediaMetadataCompat.Builder()
            .putLong(MediaMetadataCompat.METADATA_KEY_DURATION, durationMs.toLong())
            .putString(MediaMetadataCompat.METADATA_KEY_TITLE, lastTitle)
            .putString(MediaMetadataCompat.METADATA_KEY_DISPLAY_SUBTITLE, lastSubtitle)
            .build(),
        )
      }
      true
    }

    Function("setMetadata") { title: String, subtitle: String ->
      lastTitle = title
      lastSubtitle = subtitle
      true
    }

    // Playback ended or the viewer left: the projection must not keep
    // claiming a film is running.
    Function("clearPlayback") {
      session?.setPlaybackState(
        PlaybackStateCompat.Builder()
          .setState(PlaybackStateCompat.STATE_STOPPED, 0, 0f)
          .build(),
      )
      true
    }

    // Mirrors the session facts the LaunchRequestChecker reads. Called on
    // login, logout, server change and profile switch.
    Function("setSessionState") { signedIn: Boolean, serverOrigin: String, userId: String, profileId: String ->
      EveyCastPrefs.write(context, signedIn, serverOrigin, userId, profileId)
      true
    }

    // A cast received while the app is already open arrives here (the
    // activity is singleTask, so the platform reuses the instance instead of
    // stacking a second one - see withCastConnect).
    OnNewIntent { intent ->
      mediaManager?.onNewIntent(intent)
    }

    OnDestroy {
      session?.release()
      session = null
    }
  }

  private var lastTitle: String = ""
  private var lastSubtitle: String = ""

  /** Commands from the remote and from every connected sender. */
  private inner class SessionCallback : MediaSessionCompat.Callback() {
    override fun onPlay() = command("play", null)
    override fun onPause() = command("pause", null)
    override fun onStop() = command("stop", null)
    override fun onSkipToNext() = command("next", null)
    override fun onSkipToPrevious() = command("previous", null)
    override fun onSeekTo(pos: Long) = command("seek", pos)

    private fun command(kind: String, positionMs: Long?) {
      sendEvent(
        "onCastCommand",
        Bundle().apply {
          putString("kind", kind)
          if (positionMs != null) putDouble("positionMs", positionMs.toDouble())
        },
      )
    }
  }

  /** A LOAD from a sender: hand the media id to the Evey player. */
  private inner class LoadCallback : com.google.android.gms.cast.tv.media.MediaLoadCommandCallback() {
    override fun onLoad(senderId: String?, request: MediaLoadRequestData): com.google.android.gms.tasks.Task<MediaLoadRequestData> {
      val payload = Bundle().apply {
        putString("raw", request.mediaInfo?.customData?.toString() ?: customDataOf(request))
      }
      if (jsListening) sendEvent("onCastLoad", payload) else pendingLoad = payload
      // Telling the SDK the load succeeded lets it publish the session to
      // senders immediately; the player then reports the real state through
      // setPlaybackState. Refusing here would tear the session down before
      // the app had a chance to open the film.
      return com.google.android.gms.tasks.Tasks.forResult(request)
    }

    private fun customDataOf(request: MediaLoadRequestData): String {
      val custom: JSONObject? = request.customData
      return custom?.toString() ?: ""
    }
  }
}
