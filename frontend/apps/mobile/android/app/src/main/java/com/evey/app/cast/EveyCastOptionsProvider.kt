package com.evey.app.cast

import android.content.Context
import com.google.android.gms.cast.LaunchOptions
import com.google.android.gms.cast.framework.CastOptions
import com.reactnative.googlecast.GoogleCastOptionsProvider

/**
 * Cast Connect for the phone sender.
 *
 * react-native-google-cast builds its CastOptions without LaunchOptions (the
 * package has no mention of androidReceiverCompatible anywhere), so a cast
 * from the app could only ever start the Web Receiver, even on a television
 * that has Evey TV installed. This adds the one missing flag and keeps
 * everything else the package configured - the media notification, the
 * expanded controller, the receiver application id from the manifest - by
 * reading them back off its own options rather than restating them here,
 * which would rot the day the package changes one.
 *
 * The flag only ever OFFERS the Android TV path: the platform still falls
 * back to the Web Receiver when the app is absent, or when Evey TV's
 * LaunchRequestChecker declines (signed out, another instance, another
 * account). Non-Android-TV devices never see a difference.
 */
class EveyCastOptionsProvider : GoogleCastOptionsProvider() {
  override fun getCastOptions(context: Context): CastOptions {
    val base = super.getCastOptions(context)
    return CastOptions.Builder()
      .setReceiverApplicationId(base.receiverApplicationId)
      .setCastMediaOptions(base.castMediaOptions)
      .setLaunchOptions(
        LaunchOptions.Builder()
          .setAndroidReceiverCompatible(true)
          .build(),
      )
      .build()
  }
}
