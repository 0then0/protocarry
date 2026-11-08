import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import org.jetbrains.kotlin.gradle.plugin.getKotlinPluginVersion

plugins {
  kotlin("jvm") version "2.4.10"
  kotlin("plugin.serialization") version "2.4.10"
}

repositories {
  mavenCentral()
}

dependencies {
  implementation("org.jetbrains.kotlinx:kotlinx-serialization-protobuf:1.12.0")
  testImplementation(kotlin("test"))
}

java {
  sourceCompatibility = JavaVersion.VERSION_17
  targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
  compilerOptions {
    jvmTarget.set(JvmTarget.JVM_17)
  }
}

// Record the versions this build actually resolved, so the adapter can report them on stderr.
val versionsDir = layout.buildDirectory.dir("generated/versions")
val writeVersions by tasks.registering {
  val output = versionsDir.map { it.file("protocarry-versions.properties") }
  val compilerVersion = getKotlinPluginVersion()
  outputs.file(output)
  doLast {
    val resolved = configurations.runtimeClasspath.get().resolvedConfiguration.resolvedArtifacts
      .associate { "${it.moduleVersion.id.group}:${it.name}" to it.moduleVersion.id.version }
    fun version(key: String) = resolved[key] ?: error("$key was not resolved")
    output.get().asFile.apply {
      parentFile.mkdirs()
      writeText(
        "kotlin.compiler=$compilerVersion\n" +
          "kotlin.stdlib=${version("org.jetbrains.kotlin:kotlin-stdlib")}\n" +
          "kotlinx.serialization.protobuf=${version("org.jetbrains.kotlinx:kotlinx-serialization-protobuf-jvm")}\n",
      )
    }
  }
}

sourceSets.main {
  resources.srcDir(writeVersions.map { versionsDir })
}

tasks.test {
  useJUnitPlatform()
  systemProperty("protocarry.seed", layout.projectDirectory.file("seed.bin").asFile.absolutePath)
}

// A single self-contained jar lets ProtoCarry run the adapter with plain `java -jar`,
// without Gradle or network access.
tasks.jar {
  archiveFileName.set("adapter.jar")
  manifest { attributes["Main-Class"] = "protocarry.kotlincase.AdapterKt" }
  duplicatesStrategy = DuplicatesStrategy.EXCLUDE
  from(configurations.runtimeClasspath.map { classpath -> classpath.map { if (it.isDirectory) it else zipTree(it) } })
  exclude("META-INF/*.SF", "META-INF/*.DSA", "META-INF/*.RSA")
}
