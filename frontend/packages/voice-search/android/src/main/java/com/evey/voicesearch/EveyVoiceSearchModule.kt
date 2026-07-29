package com.evey.voicesearch

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.speech.RecognizerIntent
import expo.modules.kotlin.activityresult.AppContextActivityResultContract
import expo.modules.kotlin.activityresult.AppContextActivityResultLauncher
import expo.modules.kotlin.functions.Coroutine
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import java.io.Serializable

/**
 * System voice recognition for the TV remote's search box.
 *
 * Typing a title on a d-pad-driven virtual keyboard is the worst input method
 * a remote has to offer; the box's own "assistant" voice recognizer is the
 * natural one. This module launches it and returns whatever it heard.
 *
 * Deliberately uses the ACTION_RECOGNIZE_SPEECH intent (resolved via
 * startActivityForResult, wired through Expo's AppContextActivityResultCaller
 * - the sanctioned replacement for Activity.startActivityForResult inside a
 * Module, since a bare Module has no Activity of its own to call it on) and
 * NOT SpeechRecognizer. The intent hands the entire microphone session - UI,
 * the RECORD_AUDIO permission, silence detection - to whatever app resolves
 * it (the Google app / Assistant on virtually every real Android TV box).
 * This module therefore needs NO RECORD_AUDIO permission of its own. That
 * matters beyond tidiness: the mobile app's AndroidManifest.xml deliberately
 * strips RECORD_AUDIO with tools:node="remove" (it plays but never records),
 * so a module that required the permission here would lose it silently at
 * manifest-merge time and fail on every real device.
 */
class EveyVoiceSearchModule : Module() {
  private lateinit var launcher: AppContextActivityResultLauncher<RecognizeInput, String?>

  override fun definition() = ModuleDefinition {
    Name("EveyVoiceSearch")

    AsyncFunction("isAvailable") {
      recognizerAvailable(appContext.reactContext)
    }

    // Explicitly typed as `suspend () -> String?` before being handed to the
    // Coroutine DSL: a bare zero-arg lambda literal is ambiguous between the
    // Coroutine(suspend () -> R) and Coroutine(suspend (P0) -> R) overloads
    // (Kotlin 2.1.20 cannot pick one from arity alone here), so the type
    // needs to be pinned down on a named value first.
    val recognize: suspend () -> String? = {
      val context = appContext.reactContext
      if (context == null || !recognizerAvailable(context)) {
        null
      } else {
        // Any failure here (activity torn down mid-flight, launcher not yet
        // registered, the resolved app misbehaving) degrades to "nothing
        // recognized" rather than an exception: the search screen must keep
        // working from the keyboard no matter what a voice attempt did.
        runCatching { launcher.launch(RecognizeInput) }.getOrNull()
      }
    }
    AsyncFunction("recognize") Coroutine recognize

    RegisterActivityContracts {
      launcher = registerForActivityResult(RecognizeSpeechContract())
    }
  }

  /**
   * Whether anything on THIS device resolves the recognize-speech intent.
   * Boxes without Play Services (or with it stripped) have nothing to
   * resolve it, and launching an intent no app answers is itself a crash.
   */
  private fun recognizerAvailable(context: Context?): Boolean {
    if (context == null) return false
    val intent = Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH)
    return context.packageManager.queryIntentActivities(intent, 0).isNotEmpty()
  }
}

/** Activity result contracts require a Serializable input; there is none here. */
private object RecognizeInput : Serializable

private class RecognizeSpeechContract : AppContextActivityResultContract<RecognizeInput, String?> {
  override fun createIntent(context: Context, input: RecognizeInput): Intent =
    Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).apply {
      putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
      putExtra(RecognizerIntent.EXTRA_MAX_RESULTS, 1)
    }

  /** RESULT_CANCELED (back button, no match, the recognizer app giving up) all read as "nothing". */
  override fun parseResult(input: RecognizeInput, resultCode: Int, intent: Intent?): String? {
    if (resultCode != Activity.RESULT_OK) return null
    val results = intent?.getStringArrayListExtra(RecognizerIntent.EXTRA_RESULTS)
    return results?.firstOrNull()?.takeIf { it.isNotBlank() }
  }
}
