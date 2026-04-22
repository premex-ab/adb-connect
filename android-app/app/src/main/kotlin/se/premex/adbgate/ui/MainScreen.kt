package se.premex.adbgate.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import se.premex.adbgate.R
import se.premex.adbgate.adb.AdbWifi
import se.premex.adbgate.adb.PairingDiscovery
import se.premex.adbgate.adb.WifiHelper
import se.premex.adbgate.service.ServiceController

@Composable
fun MainScreen() {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    var on by remember { mutableStateOf(AdbWifi.isEnabled(context)) }
    var needsWifiPanel by remember { mutableStateOf(false) }
    var statusLine by remember { mutableStateOf<String?>(null) }

    // When the toggle is on, discover the ADB-over-Wi-Fi endpoint and show IP:port.
    LaunchedEffect(on) {
        if (!on) {
            statusLine = null
            return@LaunchedEffect
        }
        val discovery = PairingDiscovery(context)
        val ep = discovery.discoverConnect()
        statusLine = if (ep != null) "${ep.host}:${ep.port}" else null
    }

    Column(
        Modifier
            .fillMaxSize()
            .padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            "Premex ADB-gate",
            style = MaterialTheme.typography.headlineMedium,
            color = MaterialTheme.colorScheme.primary,
        )
        Text(
            stringResource(R.string.by_premex),
            style = MaterialTheme.typography.bodySmall,
        )

        Spacer(Modifier.height(32.dp))

        Switch(
            checked = on,
            onCheckedChange = { wantsOn ->
                if (wantsOn) {
                    val wifiReady = WifiHelper.isEnabled(context) || WifiHelper.tryEnable(context)
                    if (wifiReady) {
                        on = true
                        needsWifiPanel = false
                        ServiceController.start(context)
                    } else {
                        needsWifiPanel = true
                    }
                } else {
                    on = false
                    needsWifiPanel = false
                    ServiceController.stop(context)
                }
            },
        )

        Spacer(Modifier.height(8.dp))

        Text(
            if (on) stringResource(R.string.toggle_on) else stringResource(R.string.toggle_off),
            style = MaterialTheme.typography.bodyMedium,
        )

        if (on && statusLine != null) {
            Spacer(Modifier.height(4.dp))
            Text(
                statusLine!!,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.secondary,
            )
        }

        if (needsWifiPanel) {
            Spacer(Modifier.height(16.dp))
            Card {
                Column(Modifier.padding(16.dp)) {
                    Text("Wi-Fi must be on", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Wireless ADB requires Wi-Fi to be enabled. Open the system Wi-Fi panel to turn it on, then flip the toggle again.",
                    )
                    Spacer(Modifier.height(8.dp))
                    Button(onClick = { context.startActivity(WifiHelper.panelIntent()) }) {
                        Text("Open Wi-Fi settings")
                    }
                }
            }
        }
    }
}
