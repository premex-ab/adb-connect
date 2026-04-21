package se.premex.adbgate.ws

class WsBackoff(private val baseMs: Long = 500, private val capMs: Long = 60_000) {
    private var attempt = 0
    fun nextDelayMs(): Long {
        val raw = baseMs shl attempt.coerceAtMost(20)
        attempt++
        return raw.coerceAtMost(capMs)
    }
    fun reset() { attempt = 0 }
}
