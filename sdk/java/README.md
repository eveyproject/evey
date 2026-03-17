# Evey Plugin SDK — Java/Kotlin

Proto file for Evey plugin development.

## Compiled plugins (.jar)

Add to your `build.gradle`:

```groovy
plugins {
    id 'com.google.protobuf' version '0.9.4'
}

protobuf {
    protoc { artifact = "com.google.protobuf:protoc:3.25.1" }
    plugins {
        grpc { artifact = "io.grpc:protoc-gen-grpc-java:1.61.0" }
    }
}
```

The `.jar` plugin is self-contained — Evey just runs `java -jar plugin.jar`.

Requires JRE 17+ in the container (see `Dockerfile.plugins.example`).
