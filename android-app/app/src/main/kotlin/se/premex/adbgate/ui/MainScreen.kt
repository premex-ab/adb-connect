package se.premex.adbgate.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import se.premex.adbgate.R
import se.premex.adbgate.adb.PairCodeBridge
import se.premex.adbgate.adb.WifiHelper
import se.premex.adbgate.data.EncryptedConfigStore
import se.premex.adbgate.service.ServiceController

@Composable
fun MainScreen(onOpenSettings: () -> Unit) {
    val context = LocalContext.current
    val store = remember { EncryptedConfigStore(context) }
    val cfg = remember { store.load() }
    var on by remember { mutableStateOf(false) }

    Column(
        Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Premex ADB-gate", style = MaterialTheme.typography.headlineMedium, color = MaterialTheme.colorScheme.primary)
        Text(stringResource(R.string.by_premex), style = MaterialTheme.typography.bodySmall)
        Spacer(Modifier.height(32.dp))
        var needsWifiPanel by remember { mutableStateOf(false) }
        Switch(
            checked = on,
            onCheckedChange = { wantsOn ->
                if (wantsOn) {
                    val wifiReady = WifiHelper.isEnabled(context) || WifiHelper.tryEnable(context)
                    if (wifiReady) {
                        on = true
                        ServiceController.start(context)
                        needsWifiPanel = false
                    } else {
                        needsWifiPanel = true
                    }
                } else {
                    on = false
                    ServiceController.stop(context)
                }
            },
        )
        if (needsWifiPanel) {
            Spacer(Modifier.height(12.dp))
            Card {
                Column(Modifier.padding(16.dp)) {
                    Text("Wi-Fi must be on", style = MaterialTheme.typography.titleMedium)
                    Text("Wireless ADB requires Wi-Fi to be enabled. Open the system Wi-Fi panel to turn it on, then flip the toggle again.")
                    Spacer(Modifier.height(8.dp))
                    Button(onClick = { context.startActivity(WifiHelper.panelIntent()) }) { Text("Open Wi-Fi settings") }
                }
            }
        }
        Spacer(Modifier.height(12.dp))
        Text(if (on) stringResource(R.string.toggle_on) else stringResource(R.string.toggle_off))
        Spacer(Modifier.height(24.dp))
        Text("Server: ${cfg?.serverHost ?: "—"}")
        Text("Nickname: ${cfg?.nickname ?: "—"}")
        Spacer(Modifier.height(32.dp))
        TextButton(onClick = onOpenSettings) { Text(stringResource(R.string.settings)) }

        val pairPending by produceState(initialValue = PairCodeBridge.isPending()) {
            while (true) { value = PairCodeBridge.isPending(); delay(300) }
        }
        if (pairPending) {
            Spacer(Modifier.height(24.dp))
            Card {
                Column(Modifier.padding(16.dp)) {
                    Text("Pairing required", style = MaterialTheme.typography.titleMedium)
                    Text("Open Settings → Developer options → Wireless debugging → 'Pair device with pairing code', then enter the 6-digit code below.")
                    var code by remember { mutableStateOf("") }
                    Spacer(Modifier.height(8.dp))
                    OutlinedTextField(
                        value = code,
                        onValueChange = { code = it.filter(Char::isDigit).take(6) },
                        label = { Text("6-digit code") },
                        singleLine = true,
                    )
                    Spacer(Modifier.height(8.dp))
                    Button(enabled = code.length == 6, onClick = { PairCodeBridge.submit(code) }) { Text("Submit") }
                }
            }
        }
    }
}
