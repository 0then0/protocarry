@file:OptIn(ExperimentalSerializationApi::class)

package protocarry.kotlincase

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.protobuf.ProtoNumber
import kotlinx.serialization.protobuf.ProtoUnknownFieldHolder

// The "old" application schema (old.proto) as Kotlin models. Field numbers are explicit.
// New schema: Envelope.future_note = 3 and Child.future_note = 2, neither known here.

// Mode 1: no holder. Unknown fields have nowhere to go and are dropped on decode.
@Serializable
data class PlainChild(
  @ProtoNumber(1) val label: String = "",
)

@Serializable
data class PlainEnvelope(
  @ProtoNumber(1) val id: String = "",
  @ProtoNumber(2) val revision: Int = 0,
  @ProtoNumber(4) val child: PlainChild? = null,
)

// Modes 2 and 3: a holder at every level that can receive unknown fields. The holder has no
// @ProtoNumber: kotlinx.serialization recognizes it by type and fills it with the raw bytes of
// fields the model does not declare. The compiler plugin still derives a position-based number
// for it and warns if that collides with an explicit @ProtoNumber, so it is declared before
// `child` (position 3) rather than after it (position 4 would collide with child = 4).
@Serializable
data class HolderChild(
  @ProtoNumber(1) val label: String = "",
  val unknown: ProtoUnknownFieldHolder = ProtoUnknownFieldHolder.Empty,
)

@Serializable
data class HolderEnvelope(
  @ProtoNumber(1) val id: String = "",
  @ProtoNumber(2) val revision: Int = 0,
  val unknown: ProtoUnknownFieldHolder = ProtoUnknownFieldHolder.Empty,
  @ProtoNumber(4) val child: HolderChild? = null,
)
