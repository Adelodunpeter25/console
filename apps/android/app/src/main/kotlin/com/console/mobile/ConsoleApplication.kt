package com.console.mobile

import android.app.Application
import com.console.mobile.core.notification.AppForegroundTracker
import java.security.Security
import org.bouncycastle.jce.provider.BouncyCastleProvider

class ConsoleApplication : Application() {
    override fun onCreate() {
        // Insert BC so TLS/crypto works before anything else. No-op if already present.
        if (Security.getProvider(BouncyCastleProvider.PROVIDER_NAME) == null) {
            Security.insertProviderAt(BouncyCastleProvider(), 1)
        }
        super.onCreate()
        AppForegroundTracker.register(this)
        AppContainer.initialize(this)
    }
}
