# Add project specific ProGuard rules here.
# By default, the flags in this file are appended to flags specified
# in /usr/local/Cellar/android-sdk/24.3.3/tools/proguard/proguard-android.txt
# You can edit the include path and order by changing the proguardFiles
# directive in build.gradle.
#
# For more details, see
#   http://developer.android.com/guide/developing/tools/proguard.html

# react-native-reanimated
-keep class com.swmansion.reanimated.** { *; }
-keep class com.facebook.react.turbomodule.** { *; }

# Add any project specific keep options here:

# AudioBoostModule reads expo-video's private player registry reflectively to
# attach the LoudnessEnhancer to OUR player's audio session instead of the
# global mix (see AudioBoostModule.resolvePlayerSessionId). R8 must not rename
# the field, or every release build silently falls back to the global mix.
-keepclassmembers class expo.modules.video.managers.VideoManager { <fields>; }
-keep class expo.modules.video.player.VideoPlayer { *; }
# ...and the reflective hop into media3: getAudioSessionId must keep its name.
-keepclassmembers class * { int getAudioSessionId(); }

