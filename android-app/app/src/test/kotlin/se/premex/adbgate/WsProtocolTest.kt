package se.premex.adbgate

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import se.premex.adbgate.ws.ServerFrame
import se.premex.adbgate.ws.WsProtocol

class WsProtocolTest {
    @Test fun `encodes hello frame`() {
        val s = WsProtocol.encodeHello("alpha", "psk123", "0.1.0")
        assertTrue(s.contains("\"op\":\"hello\""))
        assertTrue(s.contains("\"nickname\":\"alpha\""))
        assertTrue(s.contains("\"psk\":\"psk123\""))
    }

    @Test fun `decodes ack`() {
        assertEquals(ServerFrame.Ack, WsProtocol.decodeServerFrame("""{"op":"ack"}"""))
    }

    @Test fun `decodes prep_connect with request_pair`() {
        val f = WsProtocol.decodeServerFrame("""{"op":"prep_connect","request_pair":true}""")
        assertEquals(ServerFrame.PrepConnect(requestPair = true), f)
    }

    @Test fun `decodes error frame`() {
        val f = WsProtocol.decodeServerFrame("""{"op":"error","code":"auth_failed","message":"bad psk"}""")
        assertEquals(ServerFrame.Error("auth_failed", "bad psk"), f)
    }

    @Test fun `unknown op falls through to Unknown`() {
        assertTrue(WsProtocol.decodeServerFrame("""{"op":"mystery"}""") is ServerFrame.Unknown)
    }

    @Test fun `malformed json returns Unknown`() {
        assertTrue(WsProtocol.decodeServerFrame("not json") is ServerFrame.Unknown)
    }
}
