package se.premex.adbgate.ui

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.*
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.common.InputImage
import se.premex.adbgate.data.Config
import se.premex.adbgate.data.EncryptedConfigStore
import java.util.concurrent.Executors

@Composable
fun QrScannerScreen(onScanned: () -> Unit) {
    val context = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current
    val store = remember { EncryptedConfigStore(context) }
    var hasCamera by remember {
        mutableStateOf(ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED)
    }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { hasCamera = it }
    LaunchedEffect(Unit) { if (!hasCamera) launcher.launch(Manifest.permission.CAMERA) }
    if (!hasCamera) {
        Column(Modifier.fillMaxSize().padding(32.dp)) { Text("Camera permission required to scan the enrollment QR.") }
        return
    }

    var scanned by remember { mutableStateOf(false) }
    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { ctx ->
            val previewView = PreviewView(ctx)
            val future = ProcessCameraProvider.getInstance(ctx)
            future.addListener({
                val cameraProvider = future.get()
                val preview = Preview.Builder().build().also { it.setSurfaceProvider(previewView.surfaceProvider) }
                val analyzer = ImageAnalysis.Builder()
                    .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                    .build()
                val scanner = BarcodeScanning.getClient()
                val exec = Executors.newSingleThreadExecutor()
                analyzer.setAnalyzer(exec) { imageProxy ->
                    val mediaImage = imageProxy.image
                    if (mediaImage != null && !scanned) {
                        val input = InputImage.fromMediaImage(mediaImage, imageProxy.imageInfo.rotationDegrees)
                        scanner.process(input)
                            .addOnSuccessListener { barcodes ->
                                val raw = barcodes.firstOrNull()?.rawValue
                                if (raw != null && !scanned) {
                                    scanned = true
                                    try {
                                        val nick = store.loadNickname() ?: return@addOnSuccessListener
                                        val cfg = Config.fromQrPayload(raw, nickname = nick)
                                        store.save(cfg)
                                        onScanned()
                                    } catch (e: Exception) { /* keep scanning */ scanned = false }
                                }
                            }
                            .addOnCompleteListener { imageProxy.close() }
                    } else imageProxy.close()
                }
                cameraProvider.unbindAll()
                cameraProvider.bindToLifecycle(lifecycle, CameraSelector.DEFAULT_BACK_CAMERA, preview, analyzer)
            }, ContextCompat.getMainExecutor(ctx))
            previewView
        },
    )
}
