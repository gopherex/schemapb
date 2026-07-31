"""The fluent authoring API — Python surface of the Go builder (new.go)."""

from __future__ import annotations

from typing import TYPE_CHECKING, Self, TypeVar

from schemapb._gen.schemapb import (
    Schema,
    SchemaField,
    SchemaFieldBool,
    SchemaFieldBytes,
    SchemaFieldChoice,
    SchemaFieldChoiceOption,
    SchemaFieldComputed,
    SchemaFieldDouble,
    SchemaFieldDuration,
    SchemaFieldFloat,
    SchemaFieldInt32,
    SchemaFieldInt64,
    SchemaFieldJson,
    SchemaFieldList,
    SchemaFieldMap,
    SchemaFieldObject,
    SchemaFieldOneOf,
    SchemaFieldRef,
    SchemaFieldResultType,
    SchemaFieldRule,
    SchemaFieldSeverity,
    SchemaFieldString,
    SchemaFieldTimestamp,
    SchemaFieldUInt32,
    SchemaFieldUInt64,
    SchemaIdentity,
    Value,
)
from schemapb.engine import Engine, compile_schema
from schemapb.value import int64_v, str_v

if TYPE_CHECKING:
    import datetime as dt

_N = TypeVar("_N", int, float)


class RuleB:
    def __init__(self, expr: str, message: str) -> None:
        self.r = SchemaFieldRule(expr=expr, message=message)

    def id(self, rule_id: str) -> Self:
        self.r.id = rule_id
        return self

    def warn(self) -> Self:
        self.r.severity = SchemaFieldSeverity.WARNING
        return self

    def severity(self, s: SchemaFieldSeverity) -> Self:
        self.r.severity = s
        return self

    def done(self) -> SchemaFieldRule:
        return self.r


def rule(expr: str, message: str) -> RuleB:
    return RuleB(expr, message)


class FieldB:
    def __init__(self, name: str) -> None:
        self.f = SchemaField(name=name)

    def required(self) -> Self:
        self.f.required = True
        return self

    def nullable(self) -> Self:
        self.f.nullable = True
        return self

    def immutable(self) -> Self:
        self.f.immutable = True
        return self

    def group(self, g: str) -> Self:
        self.f.group = g
        return self

    def unit(self, u: str) -> Self:
        self.f.unit = u
        return self

    def desc(self, d: str) -> Self:
        self.f.description = d
        return self

    def title(self, t: str) -> Self:
        self.f.title = t
        return self

    def deprecated(self) -> Self:
        self.f.deprecated = True
        return self

    def secret(self) -> Self:
        self.f.secret = True
        return self

    def examples(self, *vals: Value) -> Self:
        self.f.examples.extend(vals)
        return self

    def rules(self, *rs: RuleB) -> Self:
        self.f.rules.extend(r.done() for r in rs)
        return self

    def normalize(self, e: str) -> Self:
        self.f.normalize = e
        return self

    def when(self, e: str) -> Self:
        self.f.when = e
        return self

    def done(self) -> SchemaField:
        return self.f


_NUM_KINDS = {
    "float": ("float", SchemaFieldFloat),
    "double": ("double", SchemaFieldDouble),
    "int32": ("int32", SchemaFieldInt32),
    "int64": ("int64", SchemaFieldInt64),
    "uint32": ("uint32", SchemaFieldUInt32),
    "uint64": ("uint64", SchemaFieldUInt64),
}


class NumB(FieldB):
    """One generic numeric builder for all six numeric kinds."""

    def __init__(self, name: str, kind: str) -> None:
        super().__init__(name)
        attr, ctor = _NUM_KINDS[kind]
        self._k = ctor()
        setattr(self.f, attr, self._k)

    def default(self, v: _N) -> Self:
        self._k.default = v
        return self

    def const(self, v: _N) -> Self:
        self._k.const = v
        return self

    def gt(self, v: _N) -> Self:
        self._k.gt = v
        return self

    def gte(self, v: _N) -> Self:
        self._k.gte = v
        return self

    def lt(self, v: _N) -> Self:
        self._k.lt = v
        return self

    def lte(self, v: _N) -> Self:
        self._k.lte = v
        return self

    def in_(self, *vs: _N) -> Self:
        self._k.in_ = list(vs)
        return self

    def not_in(self, *vs: _N) -> Self:
        self._k.not_in = list(vs)
        return self

    def multiple_of(self, v: _N) -> Self:
        self._k.multiple_of = v
        return self


def float_(name: str) -> NumB:
    return NumB(name, "float")


def double(name: str) -> NumB:
    return NumB(name, "double")


def int32(name: str) -> NumB:
    return NumB(name, "int32")


def int64(name: str) -> NumB:
    return NumB(name, "int64")


def uint32(name: str) -> NumB:
    return NumB(name, "uint32")


def uint64(name: str) -> NumB:
    return NumB(name, "uint64")


class BoolB(FieldB):
    def __init__(self, name: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldBool()
        self.f.bool = self._k

    def default(self, v: bool) -> Self:
        self._k.default = v
        return self

    def const(self, v: bool) -> Self:
        self._k.const = v
        return self


def bool_(name: str) -> BoolB:
    return BoolB(name)


class StrB(FieldB):
    def __init__(self, name: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldString()
        self.f.string = self._k

    def default(self, v: str) -> Self:
        self._k.default = v
        return self

    def const(self, v: str) -> Self:
        self._k.const = v
        return self

    def len(self, v: int) -> Self:
        self._k.len = v
        return self

    def min_len(self, v: int) -> Self:
        self._k.min_len = v
        return self

    def max_len(self, v: int) -> Self:
        self._k.max_len = v
        return self

    def pattern(self, v: str) -> Self:
        self._k.pattern = v
        return self

    def in_(self, *vs: str) -> Self:
        self._k.in_ = list(vs)
        return self

    def not_in(self, *vs: str) -> Self:
        self._k.not_in = list(vs)
        return self

    def format(self, v: str) -> Self:
        self._k.format = v
        return self


def str_(name: str) -> StrB:
    return StrB(name)


class BytesB(FieldB):
    def __init__(self, name: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldBytes()
        self.f.bytes = self._k

    def default(self, v: bytes) -> Self:
        self._k.default = v
        return self

    def const(self, v: bytes) -> Self:
        self._k.const = v
        return self

    def len(self, v: int) -> Self:
        self._k.len = v
        return self

    def min_len(self, v: int) -> Self:
        self._k.min_len = v
        return self

    def max_len(self, v: int) -> Self:
        self._k.max_len = v
        return self

    def prefix(self, v: bytes) -> Self:
        self._k.prefix = v
        return self

    def suffix(self, v: bytes) -> Self:
        self._k.suffix = v
        return self

    def in_(self, *vs: bytes) -> Self:
        self._k.in_ = list(vs)
        return self

    def not_in(self, *vs: bytes) -> Self:
        self._k.not_in = list(vs)
        return self


def bytes_(name: str) -> BytesB:
    return BytesB(name)


class ChoiceB(FieldB):
    def __init__(self, name: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldChoice()
        self.f.choice = self._k

    def opt(self, value: Value, label: str = "") -> Self:
        self._k.options.append(SchemaFieldChoiceOption(value=value, label=label))
        return self

    def str_opts(self, *vs: str) -> Self:
        for v in vs:
            self.opt(str_v(v))
        return self

    def int_opts(self, *vs: int) -> Self:
        for v in vs:
            self.opt(int64_v(v))
        return self

    def default(self, v: Value) -> Self:
        self._k.default = v
        return self

    def open(self) -> Self:
        self._k.open = True
        return self

    def options(self, e: str) -> Self:
        self._k.options_expr = e
        return self


def choice(name: str) -> ChoiceB:
    return ChoiceB(name)


class DurationB(FieldB):
    def __init__(self, name: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldDuration()
        self.f.duration = self._k

    def default(self, d: dt.timedelta) -> Self:
        self._k.default = d
        return self

    def gt(self, d: dt.timedelta) -> Self:
        self._k.gt = d
        return self

    def gte(self, d: dt.timedelta) -> Self:
        self._k.gte = d
        return self

    def lt(self, d: dt.timedelta) -> Self:
        self._k.lt = d
        return self

    def lte(self, d: dt.timedelta) -> Self:
        self._k.lte = d
        return self


def duration(name: str) -> DurationB:
    return DurationB(name)


class TimestampB(FieldB):
    def __init__(self, name: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldTimestamp()
        self.f.timestamp = self._k

    def default(self, t: dt.datetime) -> Self:
        self._k.default = t
        return self

    def gt(self, t: dt.datetime) -> Self:
        self._k.gt = t
        return self

    def gte(self, t: dt.datetime) -> Self:
        self._k.gte = t
        return self

    def lt(self, t: dt.datetime) -> Self:
        self._k.lt = t
        return self

    def lte(self, t: dt.datetime) -> Self:
        self._k.lte = t
        return self


def timestamp(name: str) -> TimestampB:
    return TimestampB(name)


class JsonB(FieldB):
    def __init__(self, name: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldJson()
        self.f.json = self._k

    def default(self, v: Value) -> Self:
        self._k.default = v
        return self


def json_(name: str) -> JsonB:
    return JsonB(name)


class ListB(FieldB):
    def __init__(self, name: str, *items: FieldB) -> None:
        super().__init__(name)
        self._k = SchemaFieldList(items=[i.done() for i in items])
        self.f.list = self._k

    def min_items(self, v: int) -> Self:
        self._k.min_items = v
        return self

    def max_items(self, v: int) -> Self:
        self._k.max_items = v
        return self

    def unique(self) -> Self:
        self._k.unique = True
        return self

    def count(self, e: str) -> Self:
        self._k.count_expr = e
        return self


def list_(name: str, *items: FieldB) -> ListB:
    return ListB(name, *items)


class ObjectB(FieldB):
    def __init__(self, name: str, *fields: FieldB) -> None:
        super().__init__(name)
        self._sub = Schema(fields=[f.done() for f in fields])
        self.f.object = SchemaFieldObject(schema=self._sub)

    def strict(self) -> Self:
        self._sub.strict = True
        return self

    def min_props(self, n: int) -> Self:
        self._sub.min_properties = n
        return self

    def max_props(self, n: int) -> Self:
        self._sub.max_properties = n
        return self

    def rule(self, *rs: RuleB) -> Self:
        self._sub.rules.extend(r.done() for r in rs)
        return self


def object_(name: str, *fields: FieldB) -> ObjectB:
    return ObjectB(name, *fields)


class MapB(FieldB):
    def __init__(self, name: str, *value_fields: FieldB) -> None:
        super().__init__(name)
        self._sub = Schema(fields=[f.done() for f in value_fields])
        self._k = SchemaFieldMap(value_schema=self._sub)
        self.f.map = self._k

    def strict(self) -> Self:
        self._sub.strict = True
        return self

    def min_entries(self, n: int) -> Self:
        self._k.min_entries = n
        return self

    def max_entries(self, n: int) -> Self:
        self._k.max_entries = n
        return self

    def rule(self, *rs: RuleB) -> Self:
        self._sub.rules.extend(r.done() for r in rs)
        return self


def map_(name: str, *value_fields: FieldB) -> MapB:
    return MapB(name, *value_fields)


class OneOfB(FieldB):
    def __init__(self, name: str, discriminator: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldOneOf(discriminator=discriminator)
        self.f.one_of = self._k

    def variant(self, key: str, *fields: FieldB) -> Self:
        self._k.variants[key] = Schema(fields=[f.done() for f in fields])
        return self

    def variant_of(self, key: str, s: Schema) -> Self:
        self._k.variants[key] = s
        return self


def one_of(name: str, discriminator: str) -> OneOfB:
    return OneOfB(name, discriminator)


class ComputedB(FieldB):
    def __init__(self, name: str, expr: str) -> None:
        super().__init__(name)
        self._k = SchemaFieldComputed(expr=expr)
        self.f.computed = self._k

    def result(self, rt: SchemaFieldResultType) -> Self:
        self._k.result = rt
        return self


def computed(name: str, expr: str) -> ComputedB:
    return ComputedB(name, expr)


def ref(name: str, def_name: str) -> FieldB:
    b = FieldB(name)
    b.f.ref = SchemaFieldRef(name=def_name)
    return b


def ref_id(name: str, id_: SchemaIdentity) -> FieldB:
    b = FieldB(name)
    b.f.ref = SchemaFieldRef(id=id_)
    return b


class SchemaB:
    def __init__(self, id_: SchemaIdentity) -> None:
        self.s = Schema(id=id_)

    def descr(self, d: str) -> Self:
        self.s.description = d
        return self

    def strict(self) -> Self:
        self.s.strict = True
        return self

    def coerce(self) -> Self:
        self.s.coerce = True
        return self

    def min_props(self, n: int) -> Self:
        self.s.min_properties = n
        return self

    def max_props(self, n: int) -> Self:
        self.s.max_properties = n
        return self

    def template(self, name: str, tmpl: str) -> Self:
        self.s.templates[name] = tmpl
        return self

    def fields(self, *defs: FieldB) -> Self:
        self.s.fields.extend(d.done() for d in defs)
        return self

    def rules(self, *rs: RuleB) -> Self:
        self.s.rules.extend(r.done() for r in rs)
        return self

    def def_(self, name: str, *fields: FieldB) -> Self:
        self.s.defs[name] = Schema(fields=[f.done() for f in fields])
        return self

    def def_schema(self, name: str, s: Schema) -> Self:
        self.s.defs[name] = s
        return self

    def build(self, formats: dict[str, object] | None = None) -> tuple[Schema, Engine]:
        """Fully compiles the schema; SchemaError on any defect."""
        engine = compile_schema(self.s, formats=formats)
        return self.s, engine


def new_schema(id_: SchemaIdentity) -> SchemaB:
    return SchemaB(id_)
