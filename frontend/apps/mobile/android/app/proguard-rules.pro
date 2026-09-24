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

# react-native-background-downloader persists its task tables with Gson, by field name.
-keep class com.eko.RNBGDTaskConfig { *; }
-keep class com.eko.RNBGDUploadTaskConfig { *; }
-keep class com.eko.utils.StorageManager$PausedDownloadInfoData { *; }
# Same library: MMKV is compileOnly with a SharedPreferences fallback; Evey does not ship it.
-dontwarn com.tencent.mmkv.MMKV

# expo-document-picker is prebuilt against an older expo-modules-core; the annotation is gone.
-dontwarn expo.modules.kotlin.records.IsNotEmpty

# expo Record conversion finds these via kotlin-reflect; R8 -optimize strips their
# kotlin.Metadata unless kept (SecureStoreOptions cast NPE at boot, expo/expo#28010).
-keep @interface expo.modules.kotlin.records.Field { *; }
-keep @interface expo.modules.kotlin.records.Required { *; }
