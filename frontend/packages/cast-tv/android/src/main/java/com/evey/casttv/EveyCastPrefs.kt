package com.evey.casttv

import android.content.Context

/**
 * The few facts the LaunchRequestChecker needs, mirrored where NATIVE code can
 * read them.
 *
 * Why a mirror at all: the TV app keeps its server URL in expo-secure-store and
 * its session in a JS store, and the checker runs before - sometimes long
 * before - React Native has booted. A cold Chromecast launch reaches
 * onLaunchRequest with no JS bridge at all, so asking the JS side then would
 * either deadlock or, worse, answer "not signed in" for an app that is. The JS
 * side pushes these three values on every login, logout, server change and
 * profile switch (see CastConnectProvider), and the checker reads the last
 * known state.
 *
 * Nothing secret lives here: an origin, an account id and a boolean. No token,
 * no password - those stay in SecureStore.
 */
object EveyCastPrefs {
  private const val FILE = "evey_cast"
  private const val KEY_SIGNED_IN = "signedIn"
  private const val KEY_SERVER_ORIGIN = "serverOrigin"
  private const val KEY_USER_ID = "userId"
  private const val KEY_PROFILE_ID = "profileId"

  private fun prefs(context: Context) = context.getSharedPreferences(FILE, Context.MODE_PRIVATE)

  fun write(context: Context, signedIn: Boolean, serverOrigin: String?, userId: String?, profileId: String?) {
    prefs(context).edit()
      .putBoolean(KEY_SIGNED_IN, signedIn)
      .putString(KEY_SERVER_ORIGIN, serverOrigin ?: "")
      .putString(KEY_USER_ID, userId ?: "")
      .putString(KEY_PROFILE_ID, profileId ?: "")
      .apply()
  }

  fun signedIn(context: Context): Boolean = prefs(context).getBoolean(KEY_SIGNED_IN, false)

  fun serverOrigin(context: Context): String = prefs(context).getString(KEY_SERVER_ORIGIN, "") ?: ""

  fun userId(context: Context): String = prefs(context).getString(KEY_USER_ID, "") ?: ""
}
