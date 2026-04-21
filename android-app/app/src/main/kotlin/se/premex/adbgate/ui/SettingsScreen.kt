package se.premex.adbgate.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import se.premex.adbgate.R

@Composable
fun SettingsScreen(onRepair: () -> Unit, onBack: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.settings), style = MaterialTheme.typography.headlineSmall)
        Button(onClick = onRepair) { Text(stringResource(R.string.repair)) }
        TextButton(onClick = onBack) { Text("Back") }
    }
}
