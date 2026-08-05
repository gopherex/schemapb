"""Public, stable re-export of the generated protobuf types.

Consumer codegen that must reference schemapb messages from its own
generated tree points here (e.g. a betterproto2 shim module:
``from schemapb.pb import *``). The path ``schemapb.pb`` is a stability
promise; the internal ``_gen`` layout is not.
"""

from schemapb._gen.schemapb import *  # noqa: F403
