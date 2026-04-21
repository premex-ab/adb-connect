package se.premex.adbgate

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import se.premex.adbgate.ws.WsBackoff

class WsBackoffTest {
    @Test fun `first retry is at base`() {
        assertEquals(500L, WsBackoff(baseMs = 500, capMs = 60_000).nextDelayMs())
    }

    @Test fun `delay doubles up to cap`() {
        val b = WsBackoff(baseMs = 500, capMs = 60_000)
        val delays = (1..20).map { b.nextDelayMs() }
        assertEquals(500L, delays[0])
        assertEquals(1000L, delays[1])
        assertEquals(2000L, delays[2])
        assertTrue(delays.last() <= 60_000L)
        assertEquals(60_000L, delays.last())
    }

    @Test fun `reset returns delay to base`() {
        val b = WsBackoff(baseMs = 500, capMs = 60_000)
        repeat(10) { b.nextDelayMs() }
        b.reset()
        assertEquals(500L, b.nextDelayMs())
    }
}
