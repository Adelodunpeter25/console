// Top-level build file — see app/build.gradle.kts for all configuration.
plugins {
    // version catalog would live here; keeping deps inline in app/build.gradle.kts for v1
    id("com.android.application") version "8.13.0" apply false
    id("org.jetbrains.kotlin.android") version "2.2.21" apply false
    id("org.jetbrains.kotlin.plugin.compose") version "2.2.21" apply false
    id("org.jetbrains.kotlin.plugin.serialization") version "2.2.21" apply false
}
