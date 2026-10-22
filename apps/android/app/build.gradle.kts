import java.io.FileInputStream
import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
    id("com.squareup.wire")
}

val keystorePropertiesFile = rootProject.file("key.properties")
val keystoreProperties = Properties().apply {
    if (keystorePropertiesFile.exists()) {
        load(FileInputStream(keystorePropertiesFile))
    }
}
val hasReleaseSigning =
    keystorePropertiesFile.exists() &&
        listOf("storeFile", "storePassword", "keyAlias", "keyPassword")
            .all { key -> !keystoreProperties.getProperty(key).isNullOrBlank() }

android {
    namespace = "com.console.mobile"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.console.mobile"
        minSdk = 26
        targetSdk = 36
        versionCode = 3
        versionName = "1.0.1"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        ndk {
            abiFilters += listOf("arm64-v8a", "armeabi-v7a", "x86", "x86_64")
        }

        externalNativeBuild {
            cmake {
                cppFlags += "-std=c++17"
            }
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.22.1"
        }
    }

    signingConfigs {
        if (hasReleaseSigning) {
            create("release") {
                storeFile = rootProject.file(keystoreProperties["storeFile"] as String)
                storePassword = keystoreProperties["storePassword"] as String
                keyAlias = keystoreProperties["keyAlias"] as String
                keyPassword = keystoreProperties["keyPassword"] as String
            }
        }
    }

    buildTypes {
        debug {
            // Installs side by side with the release app (com.console.mobile).
            applicationIdSuffix = ".dev"
            versionNameSuffix = "-dev"
            // Sign with the fixed release key when available so a debug build
            // replaces an earlier one instead of failing with
            // INSTALL_FAILED_UPDATE_INCOMPATIBLE — the auto-generated debug
            // key differs per machine.
            if (hasReleaseSigning) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            if (hasReleaseSigning) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    sourceSets {
        // Centralized unit tests live in apps/android/tests (single folder).
        // Keep the default src/test as well so IDE wizards still work.
        getByName("test") {
            java.srcDirs("../tests", "src/test/java", "src/test/kotlin")
        }
        // Instrumented tests need a device (Cronet is a native library, so it
        // cannot run on the JVM). Flat folder alongside `tests/`, mirroring
        // the JVM suite's layout.
        getByName("androidTest") {
            java.srcDirs("../instrumented-tests", "src/androidTest/java", "src/androidTest/kotlin")
        }
    }

    packaging {
        resources {
            // The former "META-INF/versions/9/OSGI-INF/MANIFEST.MF" pickFirst was
            // removed with bcprov-jdk18on, which was the only jar in this graph
            // shipping that path. Re-add a pickFirst here if that ever changes.
        }
    }

    // Pin Compose compiler to the Kotlin version via the plugin
}

// Shared protobuf schema (docs/plan/shared-protobuf-schema.md §3): Wire reads
// .proto directly; generated Kotlin lands in build/generated/source/wire and
// is never committed. oneof -> sealed class matches ChatModels' ActivityEvent.
wire {
    sourcePath {
        srcDir("../../../proto")
    }
    kotlin {
        oneofMode = "sealed_class"
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

gradle.taskGraph.whenReady {
    val releasePackagingTaskRequested =
        allTasks.any { task ->
            task.project == project &&
                task.name.endsWith("Release") &&
                (
                    task.name.startsWith("assemble") ||
                        task.name.startsWith("bundle") ||
                        task.name.startsWith("package")
                )
        }
    if (releasePackagingTaskRequested && !hasReleaseSigning) {
        // Allow debug CI to pass without signing; fail only when explicitly
        // packaging release. Keep message so local release builds are clear.
        throw GradleException("Missing android/key.properties for release signing")
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2026.02.01")
    implementation(composeBom)
    androidTestImplementation(composeBom)

    // QR scanning for the "Add Environment" sheet: the pairing code is
    // printed by the server, and a camera beats transcribing it.
    //
    // zxing-android-embedded is one dependency with its own capture activity
    // and permission handling, which is a lot less code than wiring CameraX
    // and an image analyser by hand.
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.2")

    // QUIC/WebTransport spike (plan Phase 3). Cronet is the only Android
    // networking stack with QUIC; `cronet-embedded` is deprecated in favour
    // of `cronet-bundled`, which carries the native implementation directly.
    // The bidirectional-stream API WebTransport needs is experimental and
    // absent from the Java fallback engine, so the spike asserts the native
    // engine is the one in use.
    androidTestImplementation("org.chromium.net:cronet-bundled:500.1.0")

    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("com.google.android.material:material:1.12.0")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("androidx.navigation:navigation-compose:2.8.4")

    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.browser:browser:1.8.0")
    implementation("io.github.lyxnx.compose.ui:tabler-icons-android:3.31.0")
    implementation("com.valentinilk.shimmer:compose-shimmer:1.4.0")

    implementation("io.coil-kt.coil3:coil-compose:3.0.0")
    implementation("io.coil-kt.coil3:coil-network-okhttp:3.0.0")
    implementation("io.coil-kt.coil3:coil-svg:3.0.0")

    // Markdown parser (AST only, CommonMark + GFM) used by CustomMarkdown.
    implementation("org.jetbrains:markdown:0.7.9")

    // Code viewer — sora-editor read-only (Phase 4). Added now so viewer spike can compile.
    implementation("io.github.Rosemoe.sora-editor:editor:0.21.1")
    // Native Android LaTeX drawable for inline and display math.
    implementation("ru.noties:jlatexmath-android:0.2.0")
    // language-textmate artifact coordinates vary by publish; keep commented until Phase 4 spike
    // implementation("io.github.Rosemoe.sora-editor:language-textmate:0.21.1")

    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("androidx.security:security-crypto:1.1.0")

    // Wire protobuf runtime + Moshi JSON layer for wire types (see wire{} above).
    // kotlinx.serialization stays for persisted and non-wire types.
    implementation("com.squareup.wire:wire-runtime:7.0.4")
    implementation("com.squareup.wire:wire-moshi-adapter:7.0.4")
    implementation("com.squareup.moshi:moshi-kotlin:1.15.2")

    debugImplementation("androidx.compose.ui:ui-tooling")
    debugImplementation("androidx.compose.ui:ui-test-manifest")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlin:kotlin-test:2.4.20")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
    testImplementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
}
