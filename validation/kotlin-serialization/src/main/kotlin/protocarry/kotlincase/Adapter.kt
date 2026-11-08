package protocarry.kotlincase

import java.util.Properties
import kotlin.system.exitProcess
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.decodeFromByteArray
import kotlinx.serialization.encodeToByteArray
import kotlinx.serialization.protobuf.ProtoBuf

// ProtoCarry adapter: one unframed binary Envelope on stdin, one on stdout, logs on stderr.
// It only knows the old schema; the new schema and the assertions live in ProtoCarry.

const val MODE_NO_HOLDER = "no-holder"
const val MODE_PRESERVE = "preserve-holder"
const val MODE_REBUILD = "rebuild"
val MODES = listOf(MODE_NO_HOLDER, MODE_PRESERVE, MODE_REBUILD)

private const val EXIT_USAGE = 64

/** Rebuilds the message from its known properties. Both holders are left out, so both default to empty. */
fun HolderEnvelope.rebuildFromKnownProperties(): HolderEnvelope = HolderEnvelope(
  id = id,
  revision = revision,
  child = child?.let { HolderChild(label = it.label) },
)

@OptIn(ExperimentalSerializationApi::class)
fun transform(mode: String, input: ByteArray, log: (String) -> Unit = {}): ByteArray = when (mode) {
  MODE_NO_HOLDER -> ProtoBuf.encodeToByteArray(ProtoBuf.decodeFromByteArray<PlainEnvelope>(input))
  MODE_PRESERVE -> {
    val decoded = ProtoBuf.decodeFromByteArray<HolderEnvelope>(input)
    log("""{"unknown_bytes":{"root":${decoded.unknown.size},"child":${decoded.child?.unknown?.size ?: -1}}}""")
    ProtoBuf.encodeToByteArray(decoded)
  }
  MODE_REBUILD -> {
    val decoded = ProtoBuf.decodeFromByteArray<HolderEnvelope>(input)
    val rebuilt = decoded.rebuildFromKnownProperties()
    log(
      """{"unknown_bytes_decoded":{"root":${decoded.unknown.size},"child":${decoded.child?.unknown?.size ?: -1}},""" +
        """"unknown_bytes_rebuilt":{"root":${rebuilt.unknown.size},"child":${rebuilt.child?.unknown?.size ?: -1}}}""",
    )
    ProtoBuf.encodeToByteArray(rebuilt)
  }
  else -> throw IllegalArgumentException("unknown mode: $mode")
}

private fun runtimeVersions(): String {
  val resource = object {}.javaClass.getResourceAsStream("/protocarry-versions.properties")
  val build = Properties().also { p -> resource?.use { p.load(it) } }
  fun v(key: String) = build.getProperty(key) ?: "unknown"
  return """{"jdk":"${System.getProperty("java.runtime.version")}","jvm_vendor":"${System.getProperty("java.vendor")}",""" +
    """"kotlin_runtime":"${KotlinVersion.CURRENT}","kotlin_compiler":"${v("kotlin.compiler")}",""" +
    """"kotlinx_serialization_protobuf":"${v("kotlinx.serialization.protobuf")}"}"""
}

fun main(args: Array<String>) {
  val mode = args.singleOrNull()?.takeIf { it in MODES }
  if (mode == null) {
    System.err.println("usage: adapter.jar ${MODES.joinToString("|")}")
    exitProcess(EXIT_USAGE)
  }
  try {
    val input = System.`in`.readBytes()
    System.err.println("""{"mode":"$mode","input_bytes":${input.size},"runtime":${runtimeVersions()}}""")
    val output = transform(mode, input) { System.err.println(it) }
    System.err.println("""{"output_bytes":${output.size}}""")
    System.out.write(output)
    System.out.flush()
    // PrintStream swallows I/O errors; a failed write must not look like success.
    check(!System.out.checkError()) { "failed to write stdout" }
  } catch (e: Exception) {
    System.err.println("adapter failed: $e")
    exitProcess(1)
  }
}
