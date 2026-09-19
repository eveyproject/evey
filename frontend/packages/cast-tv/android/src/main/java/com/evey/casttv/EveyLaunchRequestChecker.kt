package com.evey.casttv

import android.content.Context
import com.google.android.gms.cast.tv.CastLaunchRequest
import com.google.android.gms.cast.tv.CastReceiverOptions
import com.google.android.gms.tasks.Task
import com.google.android.gms.tasks.Tasks
import org.json.JSONObject

/**
 * Decides whether THIS app takes the cast, before any media arrives.
 *
 * Returning false is not an error path: it is how a cast is handed back to
 * Evey's Web Receiver, which plays it perfectly well on any device. The app
 * takes over only when it can honestly do better - signed in, on the same
 * instance, for the same account - because then the television's own remote
 * drives the real Evey player, with its tracks, chapters and menus.
 *
 * The case this exists for happened on 2026-09-19: the television was signed
 * into the DEV server while the phone was casting from prod. Accepting that
 * launch would open a media id that means nothing on the TV's instance - a
 * black screen where the Web Receiver would simply have played the stream the
 * phone negotiated.
 *
 * The rules are the shared `shouldAcceptLaunch` (packages/media-caps,
 * castLaunch.ts), restated here because Kotlin cannot call it. The unit tests
 * on the TypeScript side are the specification; keep the two in step.
 */
class EveyLaunchRequestChecker(private val context: Context) : CastReceiverOptions.LaunchRequestChecker {

  override fun checkLaunchRequestSupported(launchRequest: CastLaunchRequest): Task<Boolean> {
    return Tasks.forResult(accepts(launchRequest))
  }

  private fun accepts(launchRequest: CastLaunchRequest): Boolean {
    if (!EveyCastPrefs.signedIn(context)) return false

    // setLaunchCredentialsData on the sender lands here, before any LOAD.
    val creds = parseCredentials(launchRequest.credentialsData?.credentials) ?: return false

    val tvOrigin = normalizeOrigin(EveyCastPrefs.serverOrigin(context))
    if (tvOrigin.isEmpty() || tvOrigin != creds.serverOrigin) return false

    val tvUser = EveyCastPrefs.userId(context)
    if (creds.userId.isNotEmpty() && tvUser.isNotEmpty() && creds.userId != tvUser) return false

    return true
  }

  internal data class Credentials(val serverOrigin: String, val userId: String)

  companion object {
    /** Mirror of parseLaunchCredentials: version-checked, origin required. */
    internal fun parseCredentials(raw: String?): Credentials? {
      if (raw.isNullOrBlank()) return null
      return try {
        val json = JSONObject(raw)
        if (json.optInt("v", 0) != 1) return null
        val origin = normalizeOrigin(json.optString("serverOrigin", ""))
        if (origin.isEmpty()) return null
        Credentials(origin, json.optString("userId", "").trim())
      } catch (_: Throwable) {
        // A sender we did not write can put anything in here.
        null
      }
    }

    /** Mirror of normalizeOrigin: scheme + lowercased host, no trailing slash. */
    internal fun normalizeOrigin(raw: String?): String {
      val s = (raw ?: "").trim()
      if (s.isEmpty()) return ""
      return try {
        val u = java.net.URI(s)
        val scheme = u.scheme ?: return s.trimEnd('/')
        val authority = u.authority ?: return s.trimEnd('/')
        "$scheme://${authority.lowercase()}"
      } catch (_: Throwable) {
        s.trimEnd('/')
      }
    }
  }
}
