-assumenosideeffects class android.util.Log {
    public static int v(...);
    public static int d(...);
    public static int i(...);
}
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile

# Wire protobuf: the runtime finds each generated message/enum's `ADAPTER`
# field and its @WireField members reflectively, so R8 must not rename or
# strip them (release builds crashed at startup with
# NoSuchFieldException: ADAPTER). See Wire docs, "ProGuard".
-keep class com.squareup.wire.** { *; }
-keep class console.v1.** { *; }
-keepattributes *Annotation*, Signature, InnerClasses, EnclosingMethod

# kotlinx.serialization: the library's own rule is conditional on `INSTANCE`
# surviving shrinking, so a @Serializable object used only as a type argument
# (e.g. navigation `composable<RouteOnboarding>`) lost its serializer and the
# app crashed after connecting with "Serializer for class 'm' is not found".
-keepclassmembers @kotlinx.serialization.Serializable class ** {
    public static ** INSTANCE;
    static **$Companion Companion;
    kotlinx.serialization.KSerializer serializer(...);
}

# RaTeX (LaTeX): the native library binds Java_io_ratex_RaTeXEngine_* by name and the
# display list is decoded reflectively by kotlinx.serialization. The AAR ships no
# consumer rules, so without this R8 can rename or strip them in release builds.
-keep class io.ratex.** { *; }
