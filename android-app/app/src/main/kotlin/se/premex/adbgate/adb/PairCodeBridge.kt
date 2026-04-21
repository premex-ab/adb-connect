package se.premex.adbgate.adb

import kotlinx.coroutines.CompletableDeferred

object PairCodeBridge {
    @Volatile private var pending: CompletableDeferred<String>? = null

    fun beginRequest(): CompletableDeferred<String> {
        pending?.cancel()
        val d = CompletableDeferred<String>()
        pending = d
        return d
    }

    fun submit(code: String) { pending?.complete(code); pending = null }
    fun cancel() { pending?.cancel(); pending = null }
    fun isPending(): Boolean = pending != null
}
