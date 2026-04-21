package se.premex.adbgate

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import se.premex.adbgate.data.Config
import se.premex.adbgate.data.InMemoryConfigStore

class ConfigStoreTest {
    @Test fun `parses enrollment payload`() {
        val c = Config.fromQrPayload("""{"v":1,"host":"macbook.tail-abc.ts.net","port":34567,"psk":"YWJj"}""", nickname = "alpha")
        assertEquals("alpha", c.nickname)
        assertEquals("macbook.tail-abc.ts.net", c.serverHost)
        assertEquals(34567, c.wsPort)
        assertEquals("YWJj", c.pskBase64)
    }

    @Test fun `in-memory store round-trips`() {
        val store = InMemoryConfigStore()
        assertNull(store.load())
        val c = Config("alpha", "host", 1234, "psk")
        store.save(c)
        assertEquals(c, store.load())
        store.clear()
        assertNull(store.load())
    }

    @Test(expected = IllegalArgumentException::class)
    fun `rejects unsupported version`() {
        Config.fromQrPayload("""{"v":2,"host":"x","port":1,"psk":"p"}""", nickname = "x")
    }
}
