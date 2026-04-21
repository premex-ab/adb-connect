package se.premex.adbgate.ui

import androidx.compose.runtime.Composable
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController

@Composable
fun AppNavGraph(startDestination: String) {
    val nav = rememberNavController()
    NavHost(navController = nav, startDestination = startDestination) {
        composable("onboarding") {
            OnboardingScreen(
                onReady = { nav.navigate("scanner") },
            )
        }
        composable("scanner") {
            QrScannerScreen(
                onScanned = {
                    nav.navigate("main") { popUpTo("onboarding") { inclusive = true } }
                },
            )
        }
        composable("main") { MainScreen(onOpenSettings = { nav.navigate("settings") }) }
        composable("settings") { SettingsScreen(onRepair = { nav.navigate("scanner") }, onBack = { nav.popBackStack() }) }
    }
}
