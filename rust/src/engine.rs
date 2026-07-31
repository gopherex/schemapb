//! The compiled engine: every CEL expression, regex pattern and Mustache
//! template compiled once, up front; conversions between the native model
//! and cel-interpreter values.

use std::collections::HashMap;
use std::sync::Arc;

use cel_interpreter::extractors::This;
use cel_interpreter::objects::{Key, Map as CelMap};
use cel_interpreter::{Context, Program, Value as Cel};

use crate::descriptor::{check_descriptor, join_path, nested_schemas, schema_err, SchemaError};
use crate::duration::{duration_from_nanos, duration_nanos};
use crate::formats::{core_formats, FormatRegistry};
use crate::gen::schemapb::schema::field::Kind as K;
use crate::gen::schemapb::{Schema, ValidationError};
use crate::render::{ascii_lower, ascii_upper};
use crate::value::{Native, NativeStruct, SchemaField};

/// An immutable compiled schema.
pub struct Engine {
    pub schema: Schema,
    pub formats: FormatRegistry,
    pub regexps: HashMap<String, regex::Regex>,
    programs: HashMap<String, Program>,
    asts: HashMap<String, cel_parser::Expression>,
    templates: HashMap<String, mustache::Template>,
}

/// Compiles a schema; `SchemaError` on any defect.
pub fn compile(schema: Schema, formats: FormatRegistry) -> Result<Engine, SchemaError> {
    let mut errs: Vec<ValidationError> = check_descriptor(&schema);

    let mut registry = core_formats();
    registry.extend(formats);

    let mut programs = HashMap::new();
    let mut asts = HashMap::new();
    for (src, path) in schema_exprs(&schema) {
        match Program::compile(&src) {
            Ok(p) => {
                if let Ok(ast) = cel_parser::parse(&src) {
                    asts.insert(src.clone(), ast);
                }
                programs.insert(src, p);
            }
            Err(e) => errs.push(schema_err(&path, &format!("cel: {e}"))),
        }
    }

    let mut regexps = HashMap::new();
    for (pattern, path) in schema_patterns(&schema) {
        match regex::Regex::new(&pattern) {
            Ok(re) => {
                regexps.insert(pattern, re);
            }
            Err(e) => errs.push(schema_err(&path, &format!("pattern: {e}"))),
        }
    }

    let mut templates = HashMap::new();
    for (name, src) in &schema.templates {
        match mustache::compile_str(src) {
            Ok(t) => {
                templates.insert(name.clone(), t);
            }
            Err(e) => errs.push(schema_err(
                &format!("templates.{name}"),
                &format!("mustache: {e}"),
            )),
        }
    }

    let engine = Engine {
        schema,
        formats: registry,
        regexps,
        programs,
        asts,
        templates,
    };
    if errs.is_empty() {
        errs.extend(engine.check_computed_cycles());
    }
    if errs.is_empty() {
        Ok(engine)
    } else {
        Err(SchemaError {
            result: crate::gen::schemapb::ValidationResult { errors: errs },
        })
    }
}

impl Engine {
    /// Runs a precompiled CEL plan (a sandboxed expression language — not
    /// Rust code evaluation). `Err(message)` on evaluation failure.
    pub fn eval(
        &self,
        src: &str,
        this: &Native,
        root: &NativeStruct,
        index: Option<i64>,
    ) -> Result<Native, String> {
        let Some(program) = self.programs.get(src) else {
            return Err(format!("expression not compiled: {src}"));
        };
        let mut ctx = Context::default();
        ctx.add_variable_from_value("this", native_to_cel(this));
        ctx.add_variable_from_value("root", native_to_cel(&Native::Struct(root.clone())));
        ctx.add_variable_from_value("index", Cel::Int(index.unwrap_or(0)));
        register_string_ext(&mut ctx);
        match program.execute(&ctx) {
            Ok(v) => cel_to_native(&v).ok_or_else(|| "unsupported CEL result".to_owned()),
            Err(e) => Err(e.to_string()),
        }
    }

    pub fn eval_bool(&self, src: &str, root: &NativeStruct) -> Result<bool, String> {
        match self.eval(src, &Native::Null, root, None)? {
            Native::Bool(b) => Ok(b),
            other => Err(format!("expression yields {other:?}, want bool")),
        }
    }

    /// The dotted root paths an expression reads.
    #[must_use]
    pub fn expr_deps(&self, src: &str) -> Vec<String> {
        let mut deps = Vec::new();
        if let Some(ast) = self.asts.get(src) {
            walk_deps(ast, &mut deps);
        }
        deps
    }

    #[must_use]
    pub fn render_template(
        &self,
        name: &str,
        ctx: &crate::render::RenderContext,
    ) -> Option<String> {
        let tmpl = self.templates.get(name)?;
        let mut out = Vec::new();
        tmpl.render(&mut out, ctx).ok()?;
        String::from_utf8(out).ok()
    }

    fn check_computed_cycles(&self) -> Vec<ValidationError> {
        let computed: HashMap<&str, &SchemaField> = self
            .schema
            .fields
            .iter()
            .filter(|f| matches!(f.kind.as_ref(), Some(K::Computed(_))))
            .map(|f| (f.name.as_str(), f))
            .collect();
        if computed.is_empty() {
            return Vec::new();
        }
        let deps: HashMap<&str, Vec<String>> = computed
            .iter()
            .map(|(name, f)| {
                let expr = match f.kind.as_ref() {
                    Some(K::Computed(c)) => c.expr.as_str(),
                    _ => "",
                };
                let ds = self
                    .expr_deps(expr)
                    .into_iter()
                    .filter(|d| d != name && computed.contains_key(d.as_str()))
                    .collect();
                (*name, ds)
            })
            .collect();

        fn visit<'a>(
            n: &'a str,
            deps: &'a HashMap<&str, Vec<String>>,
            color: &mut HashMap<&'a str, u8>,
        ) -> bool {
            match color.get(n) {
                Some(1) => return false,
                Some(2) => return true,
                _ => {}
            }
            color.insert(n, 1);
            for d in deps.get(n).map(Vec::as_slice).unwrap_or_default() {
                let Some((key, _)) = deps.get_key_value(d.as_str()) else {
                    continue;
                };
                if !visit(key, deps, color) {
                    return false;
                }
            }
            color.insert(n, 2);
            true
        }

        let mut color = HashMap::new();
        let mut errs = Vec::new();
        let mut names: Vec<&str> = computed.keys().copied().collect();
        names.sort_unstable();
        for name in names {
            if color.get(name) != Some(&2) && !visit(name, &deps, &mut color) {
                errs.push(schema_err(name, "computed field cycle"));
            }
        }
        errs
    }
}

/// The strings-extension subset (cel-interpreter has no built-in strings
/// extension; the remainder is a tracked gap).
fn register_string_ext(ctx: &mut Context) {
    ctx.add_function("lowerAscii", |This(s): This<Arc<String>>| ascii_lower(&s));
    ctx.add_function("upperAscii", |This(s): This<Arc<String>>| ascii_upper(&s));
    ctx.add_function("trim", |This(s): This<Arc<String>>| s.trim().to_owned());
}

// =============================================================================
// Expression / pattern collection
// =============================================================================

fn schema_exprs(s: &Schema) -> Vec<(String, String)> {
    let mut out: Vec<(String, String)> = Vec::new();
    let mut seen = std::collections::HashSet::new();
    let mut add = |src: &Option<String>, path: String, out: &mut Vec<(String, String)>| {
        if let Some(src) = src.as_deref() {
            if !src.is_empty() && seen.insert(src.to_owned()) {
                out.push((src.to_owned(), path));
            }
        }
    };

    #[allow(clippy::type_complexity)] // one-off callback used only here
    fn walk(
        fields: &[SchemaField],
        prefix: &str,
        add: &mut dyn FnMut(&Option<String>, String, &mut Vec<(String, String)>),
        out: &mut Vec<(String, String)>,
    ) {
        for f in fields {
            let path = join_path(prefix, &f.name);
            add(&f.when, format!("{path}#when"), out);
            add(&f.normalize, format!("{path}#normalize"), out);
            for (i, r) in f.rules.iter().enumerate() {
                add(&Some(r.expr.clone()), format!("{path}#rule[{i}]"), out);
            }
            match f.kind.as_ref() {
                Some(K::Computed(c)) => add(&Some(c.expr.clone()), format!("{path}#computed"), out),
                Some(K::Choice(ch)) => add(&ch.options_expr, format!("{path}#options"), out),
                Some(K::List(l)) => {
                    add(&l.count_expr, format!("{path}#count"), out);
                    walk(&l.items, &format!("{path}[]"), add, out);
                }
                _ => {}
            }
            for child in nested_schemas(f) {
                for (i, r) in child.rules.iter().enumerate() {
                    add(&Some(r.expr.clone()), format!("{path}#rule[{i}]"), out);
                }
                walk(&child.fields, &path, add, out);
            }
        }
    }

    for (i, r) in s.rules.iter().enumerate() {
        add(&Some(r.expr.clone()), format!("#rule[{i}]"), &mut out);
    }
    walk(&s.fields, "", &mut add, &mut out);
    for (name, def) in &s.defs {
        for (i, r) in def.rules.iter().enumerate() {
            add(
                &Some(r.expr.clone()),
                format!("$defs.{name}#rule[{i}]"),
                &mut out,
            );
        }
        walk(&def.fields, &format!("$defs.{name}"), &mut add, &mut out);
    }
    out
}

fn schema_patterns(s: &Schema) -> Vec<(String, String)> {
    let mut out = Vec::new();
    let mut seen = std::collections::HashSet::new();

    fn walk(
        fields: &[SchemaField],
        prefix: &str,
        seen: &mut std::collections::HashSet<String>,
        out: &mut Vec<(String, String)>,
    ) {
        for f in fields {
            let path = join_path(prefix, &f.name);
            if let Some(K::String(st)) = f.kind.as_ref() {
                if let Some(p) = st.pattern.as_deref() {
                    if !p.is_empty() && seen.insert(p.to_owned()) {
                        out.push((p.to_owned(), format!("{path}#pattern")));
                    }
                }
            }
            if let Some(K::List(l)) = f.kind.as_ref() {
                walk(&l.items, &format!("{path}[]"), seen, out);
            }
            for child in nested_schemas(f) {
                walk(&child.fields, &path, seen, out);
            }
        }
    }

    walk(&s.fields, "", &mut seen, &mut out);
    for (name, def) in &s.defs {
        walk(&def.fields, &format!("$defs.{name}"), &mut seen, &mut out);
    }
    out
}

// =============================================================================
// CEL <-> native conversion
// =============================================================================

fn native_to_cel(x: &Native) -> Cel {
    match x {
        Native::Null => Cel::Null,
        Native::Bool(b) => Cel::Bool(*b),
        Native::Int(n) => Cel::Int(*n),
        Native::Uint(n) => Cel::UInt(*n),
        Native::Double(f) => Cel::Float(*f),
        Native::Str(s) => Cel::String(Arc::new(s.clone())),
        Native::Bytes(b) => Cel::Bytes(Arc::new(b.clone())),
        Native::Duration(d) => Cel::Duration(chrono::Duration::nanoseconds(
            i64::try_from(duration_nanos(d)).unwrap_or(0),
        )),
        Native::Timestamp(t) => Cel::Timestamp(
            chrono::DateTime::from_timestamp(t.seconds, t.nanos.unsigned_abs())
                .map(Into::into)
                .unwrap_or_default(),
        ),
        Native::List(items) => Cel::List(Arc::new(items.iter().map(native_to_cel).collect())),
        Native::Struct(m) => {
            let map: HashMap<Key, Cel> = m
                .iter()
                .map(|(k, v)| (Key::String(Arc::new(k.clone())), native_to_cel(v)))
                .collect();
            Cel::Map(CelMap { map: Arc::new(map) })
        }
    }
}

fn cel_to_native(v: &Cel) -> Option<Native> {
    Some(match v {
        Cel::Null => Native::Null,
        Cel::Bool(b) => Native::Bool(*b),
        Cel::Int(n) => Native::Int(*n),
        Cel::UInt(n) => Native::Uint(*n),
        Cel::Float(f) => Native::Double(*f),
        Cel::String(s) => Native::Str(s.as_ref().clone()),
        Cel::Bytes(b) => Native::Bytes(b.as_ref().clone()),
        Cel::Duration(d) => Native::Duration(duration_from_nanos(i128::from(d.num_nanoseconds()?))),
        Cel::Timestamp(t) => Native::Timestamp(pbjson_types::Timestamp {
            seconds: t.timestamp(),
            nanos: i32::try_from(t.timestamp_subsec_nanos()).ok()?,
        }),
        Cel::List(items) => Native::List(
            items
                .iter()
                .map(cel_to_native)
                .collect::<Option<Vec<_>>>()?,
        ),
        Cel::Map(m) => {
            let mut out = NativeStruct::new();
            for (k, val) in m.map.iter() {
                let key = match k {
                    Key::String(s) => s.as_ref().clone(),
                    Key::Int(n) => n.to_string(),
                    Key::Uint(n) => n.to_string(),
                    Key::Bool(b) => b.to_string(),
                };
                out.insert(key, cel_to_native(val)?);
            }
            Native::Struct(out)
        }
        _ => return None,
    })
}

// =============================================================================
// AST dependency walk
// =============================================================================

fn walk_deps(expr: &cel_parser::Expression, deps: &mut Vec<String>) {
    use cel_parser::Expression as E;
    if let Some(path) = select_path(expr) {
        if !path.is_empty() {
            deps.push(path);
        }
        return;
    }
    match expr {
        E::Arithmetic(a, _, b) | E::Relation(a, _, b) => {
            walk_deps(a, deps);
            walk_deps(b, deps);
        }
        E::Ternary(c, a, b) => {
            walk_deps(c, deps);
            walk_deps(a, deps);
            walk_deps(b, deps);
        }
        E::Or(a, b) | E::And(a, b) => {
            walk_deps(a, deps);
            walk_deps(b, deps);
        }
        E::Unary(_, a) => walk_deps(a, deps),
        E::Member(target, member) => {
            walk_deps(target, deps);
            if let cel_parser::Member::Index(idx) = member.as_ref() {
                walk_deps(idx, deps);
            }
        }
        E::FunctionCall(f, target, args) => {
            walk_deps(f, deps);
            if let Some(t) = target {
                walk_deps(t, deps);
            }
            for a in args {
                walk_deps(a, deps);
            }
        }
        E::List(items) => {
            for a in items {
                walk_deps(a, deps);
            }
        }
        E::Map(entries) => {
            for (k, v) in entries {
                walk_deps(k, deps);
                walk_deps(v, deps);
            }
        }
        E::Atom(_) | E::Ident(_) => {}
    }
}

fn select_path(expr: &cel_parser::Expression) -> Option<String> {
    use cel_parser::{Atom, Expression as E, Member};
    match expr {
        E::Ident(name) => (name.as_ref() == "root").then(String::new),
        E::Member(target, member) => {
            let base = select_path(target)?;
            let key = match member.as_ref() {
                Member::Attribute(name) => name.as_ref().clone(),
                Member::Index(idx) => match idx.as_ref() {
                    E::Atom(Atom::String(s)) => s.as_ref().clone(),
                    _ => return None,
                },
                _ => return None,
            };
            Some(if base.is_empty() {
                key
            } else {
                format!("{base}.{key}")
            })
        }
        _ => None,
    }
}
