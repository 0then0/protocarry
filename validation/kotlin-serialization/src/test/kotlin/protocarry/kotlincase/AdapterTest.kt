@file:OptIn(ExperimentalSerializationApi::class)

package protocarry.kotlincase

import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromByteArray
import kotlinx.serialization.encodeToByteArray
import kotlinx.serialization.protobuf.ProtoBuf
import kotlinx.serialization.protobuf.ProtoNumber
import kotlinx.serialization.protobuf.ProtoUnknownFieldHolder

// The "new" schema (new.proto). The reading side of the check: it knows every field.
@Serializable
data class NewChild(
  @ProtoNumber(1) val label: String = "",
  @ProtoNumber(2) val futureNote: String = "",
)

@Serializable
data class NewEnvelope(
  @ProtoNumber(1) val id: String = "",
  @ProtoNumber(2) val revision: Int = 0,
  @ProtoNumber(3) val futureNote: String = "",
  @ProtoNumber(4) val child: NewChild? = null,
)

// Holder at the root only: the nested message is a plain model without a holder.
@Serializable
data class RootOnlyHolderEnvelope(
  @ProtoNumber(1) val id: String = "",
  @ProtoNumber(2) val revision: Int = 0,
  val unknown: ProtoUnknownFieldHolder = ProtoUnknownFieldHolder.Empty,
  @ProtoNumber(4) val child: PlainChild? = null,
)

// Additional in-process confirmation. The oracle remains ProtoCarry's independent Go decoder;
// this test decodes each adapter result again with the new Kotlin model.
class AdapterTest {
  private val seed = File(System.getProperty("protocarry.seed")).readBytes()
  private val original = ProtoBuf.decodeFromByteArray<NewEnvelope>(seed)

  private fun result(mode: String) = ProtoBuf.decodeFromByteArray<NewEnvelope>(transform(mode, seed))

  @Test
  fun seedCarriesTheNewFields() {
    assertEquals(NewEnvelope("A", 7, "future-root", NewChild("seed", "future-child")), original)
  }

  @Test
  fun withoutHolderUnknownFieldsAreLostAndControlsSurvive() {
    val actual = result(MODE_NO_HOLDER)
    assertEquals(NewEnvelope("A", 7, "", NewChild("seed", "")), actual)
  }

  @Test
  fun carriedHolderPreservesRootAndNestedFields() {
    assertEquals(original, result(MODE_PRESERVE))
  }

  @Test
  fun rebuildingFromKnownPropertiesLosesBothHolders() {
    val decoded = ProtoBuf.decodeFromByteArray<HolderEnvelope>(seed)
    assertTrue(decoded.unknown.size > 0 && decoded.child!!.unknown.size > 0, "holders were filled on decode")
    val rebuilt = decoded.rebuildFromKnownProperties()
    assertEquals(0, rebuilt.unknown.size)
    assertEquals(0, rebuilt.child!!.unknown.size)
    assertEquals(NewEnvelope("A", 7, "", NewChild("seed", "")), result(MODE_REBUILD))
  }

  @Test
  fun copyKeepsTheHolderUnlikeRebuilding() {
    val decoded = ProtoBuf.decodeFromByteArray<HolderEnvelope>(seed)
    val copied = decoded.copy(revision = 8)
    assertEquals(decoded.unknown, copied.unknown)
    assertEquals(decoded.child, copied.child)
  }

  @Test
  fun nestedModelNeedsItsOwnHolder() {
    val rootOnly = ProtoBuf.decodeFromByteArray<RootOnlyHolderEnvelope>(seed)
    val partial = ProtoBuf.decodeFromByteArray<NewEnvelope>(ProtoBuf.encodeToByteArray(rootOnly))
    assertEquals("future-root", partial.futureNote)
    assertEquals(NewChild("seed", ""), partial.child)
    assertNotEquals(original, partial)
  }
}
