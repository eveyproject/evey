# Image for TestRuntimeLaunchers (scripts/plugin-runtimes-itest.sh): a Debian
# with Go, plus a JRE and a .NET runtime added the way
# docker/Dockerfile.plugins.example adds them to the server image, and the two
# reference plugins (a .jar, a .dll) under /fixtures.
ARG BASE=golang:1.27-trixie

FROM eclipse-temurin:21-jdk AS javabuild
WORKDIR /src
COPY java/ .
RUN javac Plugin.java && printf 'Main-Class: Plugin\n' > manifest.txt \
    && jar cfm plugin.jar manifest.txt Plugin.class

FROM mcr.microsoft.com/dotnet/sdk:8.0 AS dotnetbuild
WORKDIR /src
COPY dotnet/ .
RUN dotnet publish -c Release -o /out

FROM ${BASE}
RUN apt-get update && apt-get install -y --no-install-recommends bubblewrap openjdk-21-jre-headless \
    && curl -fsSL https://packages.microsoft.com/config/debian/13/packages-microsoft-prod.deb -o /tmp/ms.deb \
    && dpkg -i /tmp/ms.deb && rm /tmp/ms.deb \
    && apt-get update && apt-get install -y --no-install-recommends dotnet-runtime-8.0 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=javabuild /src/plugin.jar /fixtures/java/plugin.jar
COPY --from=dotnetbuild /out/ /fixtures/dotnet/
