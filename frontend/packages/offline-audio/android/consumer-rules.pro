# gomobile's JNI bridge resolves these by name at runtime; R8 must not
# strip or rename them (same rules the gomobile AAR ships as proguard.txt).
-keep class go.** { *; }
-keep class com.evey.offlineaudio.** { *; }
