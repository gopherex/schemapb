//! `#[derive(Reflect)]` and `#[derive(Validate)]`: a Rust struct or enum
//! reflected into a schemapb Schema, and validated against it.
//!
//! The macros generate an impl of `schemapb::reflect::ReflectField` (and,
//! for `Validate`, of `schemapb::Validate`); the runtime side lives in the
//! `schemapb` crate. Every attribute value is a Rust expression typed by the
//! field, so an attribute that does not apply, or a literal that does not
//! fit, is a compile error at the attribute:
//!
//! ```text
//! #[derive(Validate, Serialize, Deserialize)]
//! #[serde(rename_all = "camelCase")]
//! #[schemapb(rule("root.min <= root.max", message = "min above max", id = "range"), strict)]
//! struct Limits {
//!     #[schemapb(gte = 1, lte = 64, default = 4, desc = "worker threads")]
//!     workers: u32,
//!     #[schemapb(format = "email")]
//!     owner: String,
//!     #[schemapb(min = 1, unique, items(pattern = "^[a-z]+$", max = 32))]
//!     tags: Vec<String>,
//!     #[schemapb(keys(pattern = "^[a-z]+$"), values(gte = 0))]
//!     quotas: HashMap<String, i64>,
//!     #[schemapb(rule("this != 0.0", message = "zero"), finite)]
//!     ratio: f64,
//!     min: u32,
//!     max: u32,
//! }
//! ```
//!
//! Field attributes: `required`, `secret`, `skip`, `unique`, `finite`,
//! `deprecated`, `immutable`, `nullable`; `gt`, `gte`, `lt`, `lte`, `min`,
//! `max`, `len`, `default`, `example` (expressions); `pattern`, `format`,
//! `desc`, `title`, `unit`, `group`, `normalize`, `when`, `rename`
//! (strings); `rule(expr, message = .., id = .., warn)`, `oneof(..)`,
//! `annotation(key, value)`; `items(..)`, `keys(..)`, `values(..)` holding
//! constraints for list items, map keys and map values. Container
//! attributes: `rule(..)`, `strict`, and for `Validate` the schema identity
//! `namespace`, `name`, `version` (defaults: the crate name, the type name).
//!
//! serde's `rename`, `rename_all`, `skip` (and `skip_serializing`),
//! `transparent` and an internally tagged enum's `tag` shape the schema like
//! they shape the data; `flatten`, `untagged` and adjacent tagging are
//! compile errors.

use proc_macro::TokenStream;
use proc_macro2::{Span, TokenStream as TokenStream2};
use quote::{quote, quote_spanned};
use syn::parse::Parse;
use syn::spanned::Spanned;
use syn::{
    parse_macro_input, Attribute, Data, DataEnum, DeriveInput, Expr, Fields, Ident, Lit, LitStr,
    Token, Type,
};

#[proc_macro_derive(Reflect, attributes(schemapb))]
pub fn derive_reflect(input: TokenStream) -> TokenStream {
    let input = parse_macro_input!(input as DeriveInput);
    reflect(&input)
        .unwrap_or_else(syn::Error::into_compile_error)
        .into()
}

#[proc_macro_derive(Validate, attributes(schemapb))]
pub fn derive_validate(input: TokenStream) -> TokenStream {
    let input = parse_macro_input!(input as DeriveInput);
    reflect(&input)
        .and_then(|r| validate(&input).map(|v| quote! { #r #v }))
        .unwrap_or_else(syn::Error::into_compile_error)
        .into()
}

// =============================================================================
// Attributes
// =============================================================================

/// A constraint key and its value (an expression, typed by the field at
/// the call site).
struct Constraint {
    key: Ident,
    value: Option<Expr>,
}

/// One CEL rule.
struct RuleAttr {
    span: Span,
    expr: LitStr,
    message: LitStr,
    id: Option<LitStr>,
    warn: bool,
}

/// What a field (or an item / key / value position) is constrained by.
#[derive(Default)]
struct Constraints {
    list: Vec<Constraint>,
    rules: Vec<RuleAttr>,
    oneof_str: Vec<LitStr>,
    oneof_int: Vec<Expr>,
    annotations: Vec<(LitStr, Expr)>,
    items: Option<Box<Self>>,
    keys: Option<Box<Self>>,
    values: Option<Box<Self>>,
}

#[derive(Default)]
struct FieldAttrs {
    rename: Option<String>,
    skip: bool,
    required: bool,
    constraints: Constraints,
}

#[derive(Default)]
struct ContainerAttrs {
    rules: Vec<RuleAttr>,
    strict: bool,
    namespace: Option<LitStr>,
    name: Option<LitStr>,
    version: Option<LitStr>,
    rename_all: Option<RenameRule>,
    tag: Option<String>,
    transparent: bool,
}

/// Keys whose value is an expression.
const VALUED: &[&str] = &[
    "gt",
    "gte",
    "lt",
    "lte",
    "min",
    "max",
    "len",
    "default",
    "example",
    "pattern",
    "format",
    "desc",
    "description",
    "title",
    "unit",
    "group",
    "normalize",
    "when",
];

/// Flag keys of a position.
const FLAGS: &[&str] = &[
    "unique",
    "finite",
    "secret",
    "deprecated",
    "immutable",
    "nullable",
];

fn parse_rule(meta: &syn::meta::ParseNestedMeta) -> syn::Result<RuleAttr> {
    let span = meta.path.span();
    let content;
    syn::parenthesized!(content in meta.input);
    let expr: LitStr = content.parse()?;
    let mut rule = RuleAttr {
        span,
        expr,
        message: LitStr::new("", span),
        id: None,
        warn: false,
    };
    let mut has_message = false;
    while !content.is_empty() {
        content.parse::<Token![,]>()?;
        if content.is_empty() {
            break;
        }
        let key: Ident = content.parse()?;
        match key.to_string().as_str() {
            "message" => {
                content.parse::<Token![=]>()?;
                rule.message = content.parse()?;
                has_message = true;
            }
            "id" => {
                content.parse::<Token![=]>()?;
                rule.id = Some(content.parse()?);
            }
            "warn" => rule.warn = true,
            other => {
                return Err(syn::Error::new(
                    key.span(),
                    format!("unknown rule option {other:?} (message, id, warn)"),
                ))
            }
        }
    }
    if !has_message {
        return Err(syn::Error::new(span, "a rule needs message = \"...\""));
    }
    Ok(rule)
}

fn parse_oneof(meta: &syn::meta::ParseNestedMeta, c: &mut Constraints) -> syn::Result<()> {
    let content;
    syn::parenthesized!(content in meta.input);
    let items = content.parse_terminated(Expr::parse, Token![,])?;
    for item in items {
        match &item {
            Expr::Lit(syn::ExprLit {
                lit: Lit::Str(s), ..
            }) => c.oneof_str.push(s.clone()),
            _ => c.oneof_int.push(item),
        }
    }
    if !c.oneof_str.is_empty() && !c.oneof_int.is_empty() {
        return Err(meta.error("oneof options must be all strings or all integers"));
    }
    Ok(())
}

/// Parses one constraint key of a position; `false` when the key is not a
/// constraint (the caller handles it).
fn parse_constraint(meta: &syn::meta::ParseNestedMeta, c: &mut Constraints) -> syn::Result<bool> {
    let Some(key) = meta.path.get_ident().cloned() else {
        return Ok(false);
    };
    let name = key.to_string();
    if VALUED.contains(&name.as_str()) {
        let value: Expr = meta.value()?.parse()?;
        c.list.push(Constraint {
            key,
            value: Some(value),
        });
        return Ok(true);
    }
    if FLAGS.contains(&name.as_str()) {
        c.list.push(Constraint { key, value: None });
        return Ok(true);
    }
    match name.as_str() {
        "rule" => c.rules.push(parse_rule(meta)?),
        "oneof" => parse_oneof(meta, c)?,
        "annotation" => {
            let content;
            syn::parenthesized!(content in meta.input);
            let k: LitStr = content.parse()?;
            content.parse::<Token![,]>()?;
            let v: Expr = content.parse()?;
            c.annotations.push((k, v));
        }
        "items" | "keys" | "values" => {
            let mut inner = Constraints::default();
            meta.parse_nested_meta(|m| {
                if parse_constraint(&m, &mut inner)? {
                    Ok(())
                } else {
                    Err(m.error("not a constraint of an item, key or value"))
                }
            })?;
            let slot = match name.as_str() {
                "items" => &mut c.items,
                "keys" => &mut c.keys,
                _ => &mut c.values,
            };
            *slot = Some(Box::new(inner));
        }
        _ => return Ok(false),
    }
    Ok(true)
}

fn field_attrs(attrs: &[Attribute]) -> syn::Result<FieldAttrs> {
    let mut out = FieldAttrs::default();
    for attr in attrs {
        if attr.path().is_ident("schemapb") {
            attr.parse_nested_meta(|meta| {
                if parse_constraint(&meta, &mut out.constraints)? {
                    return Ok(());
                }
                let key = meta
                    .path
                    .get_ident()
                    .map(ToString::to_string)
                    .unwrap_or_default();
                match key.as_str() {
                    "required" => out.required = true,
                    "skip" => out.skip = true,
                    "rename" => out.rename = Some(meta.value()?.parse::<LitStr>()?.value()),
                    other => {
                        return Err(meta.error(format!("unknown schemapb attribute {other:?}")))
                    }
                }
                Ok(())
            })?;
        } else if attr.path().is_ident("serde") {
            attr.parse_nested_meta(|meta| {
                let key = meta
                    .path
                    .get_ident()
                    .map(ToString::to_string)
                    .unwrap_or_default();
                match key.as_str() {
                    "rename" => {
                        if meta.input.peek(Token![=]) {
                            let name = meta.value()?.parse::<LitStr>()?.value();
                            out.rename.get_or_insert(name);
                        } else {
                            skip_meta(&meta)?;
                        }
                    }
                    "skip" | "skip_serializing" => out.skip = true,
                    "flatten" => return Err(meta.error(
                        "schemapb: #[serde(flatten)] is not supported; nest the struct as a field",
                    )),
                    _ => skip_meta(&meta)?,
                }
                Ok(())
            })?;
        }
    }
    Ok(out)
}

/// Consumes a serde attribute schemapb does not need.
fn skip_meta(meta: &syn::meta::ParseNestedMeta) -> syn::Result<()> {
    if meta.input.peek(Token![=]) {
        meta.value()?.parse::<Expr>()?;
    } else if meta.input.peek(syn::token::Paren) {
        meta.parse_nested_meta(|m| skip_meta(&m))?;
    }
    Ok(())
}

fn container_attrs(attrs: &[Attribute]) -> syn::Result<ContainerAttrs> {
    let mut out = ContainerAttrs::default();
    for attr in attrs {
        if attr.path().is_ident("schemapb") {
            attr.parse_nested_meta(|meta| {
                let key = meta
                    .path
                    .get_ident()
                    .map(ToString::to_string)
                    .unwrap_or_default();
                match key.as_str() {
                    "rule" => out.rules.push(parse_rule(&meta)?),
                    "strict" => out.strict = true,
                    "namespace" => out.namespace = Some(meta.value()?.parse()?),
                    "name" => out.name = Some(meta.value()?.parse()?),
                    "version" => out.version = Some(meta.value()?.parse()?),
                    other => {
                        return Err(meta.error(format!(
                            "unknown schemapb container attribute {other:?} (rule, strict, namespace, name, version)"
                        )))
                    }
                }
                Ok(())
            })?;
        } else if attr.path().is_ident("serde") {
            attr.parse_nested_meta(|meta| {
                let key = meta
                    .path
                    .get_ident()
                    .map(ToString::to_string)
                    .unwrap_or_default();
                match key.as_str() {
                    "rename_all" => {
                        if meta.input.peek(Token![=]) {
                            let lit: LitStr = meta.value()?.parse()?;
                            out.rename_all = Some(RenameRule::parse(&lit)?);
                        } else {
                            return Err(meta.error(
                                "schemapb: rename_all(serialize = .., deserialize = ..) is not supported; use one rule",
                            ));
                        }
                    }
                    "tag" => out.tag = Some(meta.value()?.parse::<LitStr>()?.value()),
                    "transparent" => out.transparent = true,
                    "untagged" => {
                        return Err(meta.error(
                            "schemapb: untagged enums have no schema shape; use #[serde(tag = \"...\")]",
                        ))
                    }
                    "content" => {
                        return Err(meta.error(
                            "schemapb: adjacently tagged enums are not supported; use #[serde(tag = \"...\")] alone",
                        ))
                    }
                    _ => skip_meta(&meta)?,
                }
                Ok(())
            })?;
        }
    }
    Ok(out)
}

// =============================================================================
// serde renaming
// =============================================================================

#[derive(Clone, Copy)]
enum RenameRule {
    Lower,
    Upper,
    Pascal,
    Camel,
    Snake,
    ScreamingSnake,
    Kebab,
    ScreamingKebab,
}

impl RenameRule {
    fn parse(lit: &LitStr) -> syn::Result<Self> {
        Ok(match lit.value().as_str() {
            "lowercase" => Self::Lower,
            "UPPERCASE" => Self::Upper,
            "PascalCase" => Self::Pascal,
            "camelCase" => Self::Camel,
            "snake_case" => Self::Snake,
            "SCREAMING_SNAKE_CASE" => Self::ScreamingSnake,
            "kebab-case" => Self::Kebab,
            "SCREAMING-KEBAB-CASE" => Self::ScreamingKebab,
            other => {
                return Err(syn::Error::new(
                    lit.span(),
                    format!("unknown rename rule {other:?}"),
                ))
            }
        })
    }

    /// serde's rule for a field (a `snake_case` identifier).
    fn field(self, name: &str) -> String {
        match self {
            Self::Lower | Self::Snake => name.to_owned(),
            Self::Upper | Self::ScreamingSnake => name.to_ascii_uppercase(),
            Self::Pascal => pascal(name),
            Self::Camel => lower_first(&pascal(name)),
            Self::Kebab => name.replace('_', "-"),
            Self::ScreamingKebab => name.to_ascii_uppercase().replace('_', "-"),
        }
    }

    /// serde's rule for a variant (a `PascalCase` identifier).
    fn variant(self, name: &str) -> String {
        match self {
            Self::Pascal => name.to_owned(),
            Self::Lower => name.to_ascii_lowercase(),
            Self::Upper => name.to_ascii_uppercase(),
            Self::Camel => lower_first(name),
            Self::Snake => snake(name),
            Self::ScreamingSnake => snake(name).to_ascii_uppercase(),
            Self::Kebab => snake(name).replace('_', "-"),
            Self::ScreamingKebab => snake(name).to_ascii_uppercase().replace('_', "-"),
        }
    }
}

fn lower_first(s: &str) -> String {
    let mut chars = s.chars();
    chars.next().map_or_else(String::new, |c| {
        c.to_ascii_lowercase().to_string() + chars.as_str()
    })
}

fn pascal(snake: &str) -> String {
    snake
        .split('_')
        .map(|part| {
            let mut chars = part.chars();
            chars.next().map_or_else(String::new, |c| {
                c.to_ascii_uppercase().to_string() + chars.as_str()
            })
        })
        .collect()
}

fn snake(pascal: &str) -> String {
    let mut out = String::new();
    for (i, c) in pascal.char_indices() {
        if i > 0 && c.is_uppercase() {
            out.push('_');
        }
        out.push(c.to_ascii_lowercase());
    }
    out
}

/// The wire name of a Rust identifier (raw identifiers lose `r#`).
fn ident_name(ident: &Ident) -> String {
    let s = ident.to_string();
    s.strip_prefix("r#")
        .map_or_else(|| s.clone(), ToOwned::to_owned)
}

// =============================================================================
// Code generation
// =============================================================================

fn rule_tokens(r: &RuleAttr) -> TokenStream2 {
    let (expr, message, warn) = (&r.expr, &r.message, r.warn);
    let id = r.id.as_ref().map_or_else(
        || quote! { ::std::option::Option::None },
        |id| quote! { ::std::option::Option::Some(#id) },
    );
    quote_spanned! {r.span=> ::schemapb::reflect::attr::rule_of(#expr, #message, #id, #warn) }
}

/// The statements applying `c` to the field `f` (a `&mut Field`) of type
/// `ty`.
fn constraint_tokens(c: &Constraints, ty: &TokenStream2) -> TokenStream2 {
    let mut out = TokenStream2::new();
    for k in &c.list {
        let span = k.key.span();
        let key = k.key.to_string();
        let stmt = match (key.as_str(), &k.value) {
            ("desc" | "description", Some(v)) => {
                quote_spanned! {span=> f.description = ::std::option::Option::Some(::std::string::String::from(#v)); }
            }
            ("title" | "unit" | "group" | "normalize" | "when", Some(v)) => {
                let field = Ident::new(&key, span);
                quote_spanned! {span=> f.#field = ::std::option::Option::Some(::std::string::String::from(#v)); }
            }
            ("example", Some(v)) => {
                quote_spanned! {span=> ::schemapb::reflect::attr::example(f, #v); }
            }
            ("secret" | "deprecated" | "immutable" | "nullable", None) => {
                let field = Ident::new(&key, span);
                quote_spanned! {span=> f.#field = true; }
            }
            (_, Some(v)) => {
                let func = Ident::new(&key, span);
                quote_spanned! {span=> ::schemapb::reflect::attr::#func::<#ty>(f, #v); }
            }
            (_, None) => {
                let func = Ident::new(&key, span);
                quote_spanned! {span=> ::schemapb::reflect::attr::#func::<#ty>(f); }
            }
        };
        out.extend(stmt);
    }
    for r in &c.rules {
        let rule = rule_tokens(r);
        out.extend(quote! { f.rules.push(#rule); });
    }
    for (k, v) in &c.annotations {
        out.extend(quote_spanned! {k.span()=> ::schemapb::reflect::attr::annotation(f, #k, #v); });
    }
    if let Some(items) = &c.items {
        let inner_ty = quote! { <#ty as ::schemapb::reflect::attr::Items>::Item };
        let body = constraint_tokens(items, &inner_ty);
        out.extend(quote! {
            ::schemapb::reflect::attr::items::<#ty>(f, |f: &mut ::schemapb::gen::schemapb::schema::Field| { #body });
        });
    }
    if let Some(keys) = &c.keys {
        let body = constraint_tokens(keys, &quote! { ::std::string::String });
        out.extend(quote! {
            ::schemapb::reflect::attr::keys::<#ty>(f, |f: &mut ::schemapb::gen::schemapb::schema::Field| { #body });
        });
    }
    if let Some(values) = &c.values {
        let inner_ty = quote! { <#ty as ::schemapb::reflect::attr::Entries>::Value };
        let body = constraint_tokens(values, &inner_ty);
        out.extend(quote! {
            ::schemapb::reflect::attr::values::<#ty>(f, |f: &mut ::schemapb::gen::schemapb::schema::Field| { #body });
        });
    }
    out
}

/// The expression building the field of `ty` named `name`.
fn field_tokens(name: &str, ty: &Type, attrs: &FieldAttrs) -> TokenStream2 {
    let c = &attrs.constraints;
    // Choice options replace the kind before presence and the remaining
    // constraints apply.
    let choice = if !c.oneof_str.is_empty() {
        let opts = &c.oneof_str;
        quote_spanned! {ty.span()=> ::schemapb::reflect::attr::choice_str::<#ty>(f, &[#(#opts),*]); }
    } else if !c.oneof_int.is_empty() {
        let opts = &c.oneof_int;
        quote_spanned! {ty.span()=> ::schemapb::reflect::attr::choice_int::<#ty>(f, &[#(::std::primitive::i64::from(#opts)),*]); }
    } else {
        TokenStream2::new()
    };
    let presence = if attrs.required {
        quote! { f.required = true; }
    } else {
        quote! { ::schemapb::reflect::attr::presence::<#ty>(f); }
    };
    let rest = constraint_tokens(c, &quote! { #ty });
    quote! {
        {
            let mut field = <#ty as ::schemapb::reflect::ReflectField>::field(#name);
            {
                let f = &mut field;
                #choice
                #presence
                #rest
            }
            field
        }
    }
}

/// Every type parameter reflects and lives as long as a schema does.
fn add_bounds(generics: &syn::Generics) -> syn::Generics {
    let mut g = generics.clone();
    for p in &mut g.params {
        if let syn::GenericParam::Type(t) = p {
            t.bounds
                .push(syn::parse_quote!(::schemapb::reflect::ReflectField));
            t.bounds.push(syn::parse_quote!('static));
        }
    }
    g
}

fn decorate_tokens(c: &ContainerAttrs) -> TokenStream2 {
    if c.rules.is_empty() && !c.strict {
        return TokenStream2::new();
    }
    let rules = c.rules.iter().map(rule_tokens);
    let strict = c.strict.then(|| quote! { schema.strict = true; });
    quote! {
        fn decorate_schema(schema: &mut ::schemapb::gen::schemapb::Schema) {
            #(schema.rules.push(#rules);)*
            #strict
        }
    }
}

fn reflect(input: &DeriveInput) -> syn::Result<TokenStream2> {
    let attrs = container_attrs(&input.attrs)?;
    let ident = &input.ident;
    let generics = add_bounds(&input.generics);
    let (impl_generics, ty_generics, where_clause) = generics.split_for_impl();

    let body = match &input.data {
        Data::Struct(data) => struct_body(data, &attrs)?,
        Data::Enum(data) => enum_body(data, &attrs)?,
        Data::Union(_) => {
            return Err(syn::Error::new_spanned(
                input,
                "schemapb: unions have no schema shape",
            ))
        }
    };

    let default_impl = match &input.data {
        Data::Enum(data) if attrs.tag.is_none() => {
            Some(enum_default(data, &attrs, ident, &generics)?)
        }
        _ => None,
    };

    Ok(quote! {
        impl #impl_generics ::schemapb::reflect::ReflectField for #ident #ty_generics #where_clause {
            #body
        }
        #default_impl
    })
}

fn struct_body(data: &syn::DataStruct, attrs: &ContainerAttrs) -> syn::Result<TokenStream2> {
    let decorate = decorate_tokens(attrs);
    if attrs.transparent {
        let field = data.fields.iter().next().ok_or_else(|| {
            syn::Error::new(
                Span::call_site(),
                "schemapb: a transparent struct needs a field",
            )
        })?;
        let ty = &field.ty;
        return Ok(quote! {
            const REQUIRED: bool = <#ty as ::schemapb::reflect::ReflectField>::REQUIRED;

            fn field(name: &str) -> ::schemapb::gen::schemapb::schema::Field {
                <#ty as ::schemapb::reflect::ReflectField>::field(name)
            }

            fn object_fields() -> ::std::option::Option<::std::vec::Vec<::schemapb::gen::schemapb::schema::Field>> {
                <#ty as ::schemapb::reflect::ReflectField>::object_fields()
            }
        });
    }

    let Fields::Named(named) = &data.fields else {
        return Err(syn::Error::new(
            data.fields.span(),
            "schemapb: a struct needs named fields (or #[serde(transparent)])",
        ));
    };

    let mut fields = Vec::new();
    for field in &named.named {
        let fa = field_attrs(&field.attrs)?;
        if fa.skip {
            continue;
        }
        let ident = field.ident.as_ref().expect("named field");
        let name = fa.rename.clone().unwrap_or_else(|| {
            let raw = ident_name(ident);
            attrs
                .rename_all
                .map_or_else(|| raw.clone(), |r| r.field(&raw))
        });
        fields.push(field_tokens(&name, &field.ty, &fa));
    }

    Ok(quote! {
        const SECTION: bool = true;

        fn field(name: &str) -> ::schemapb::gen::schemapb::schema::Field {
            ::schemapb::reflect::visiting(
                ::std::any::TypeId::of::<Self>(),
                || ::schemapb::reflect::object_field_of::<Self>(
                    name,
                    <Self as ::schemapb::reflect::ReflectField>::object_fields()
                        .unwrap_or_default(),
                ),
            )
            .unwrap_or_else(|| ::schemapb::reflect::json_field(name))
        }

        fn object_fields() -> ::std::option::Option<::std::vec::Vec<::schemapb::gen::schemapb::schema::Field>> {
            ::std::option::Option::Some(::std::vec![#(#fields),*])
        }

        #decorate
    })
}

/// A variant's wire name; `None` when serde skips it.
fn variant_name(v: &syn::Variant, attrs: &ContainerAttrs) -> syn::Result<Option<String>> {
    let fa = field_attrs(&v.attrs)?;
    if fa.skip {
        return Ok(None);
    }
    Ok(Some(fa.rename.unwrap_or_else(|| {
        let raw = ident_name(&v.ident);
        attrs
            .rename_all
            .map_or_else(|| raw.clone(), |r| r.variant(&raw))
    })))
}

fn enum_body(data: &DataEnum, attrs: &ContainerAttrs) -> syn::Result<TokenStream2> {
    let Some(tag) = &attrs.tag else {
        // Unit variants: a string Choice of the wire names.
        let mut names = Vec::new();
        for v in &data.variants {
            if !matches!(v.fields, Fields::Unit) {
                return Err(syn::Error::new(
                    v.span(),
                    "schemapb: an enum with data needs #[serde(tag = \"...\")] (internally tagged)",
                ));
            }
            if let Some(name) = variant_name(v, attrs)? {
                names.push(name);
            }
        }
        return Ok(quote! {
            fn field(name: &str) -> ::schemapb::gen::schemapb::schema::Field {
                let mut f = <::std::string::String as ::schemapb::reflect::ReflectField>::field(name);
                ::schemapb::reflect::attr::choice_str::<::std::string::String>(&mut f, &[#(#names),*]);
                f
            }
        });
    };

    let mut variants = Vec::new();
    for v in &data.variants {
        let Some(name) = variant_name(v, attrs)? else {
            continue;
        };
        let fields = match &v.fields {
            Fields::Unit => Vec::new(),
            Fields::Named(named) => {
                let mut out = Vec::new();
                for field in &named.named {
                    let fa = field_attrs(&field.attrs)?;
                    if fa.skip {
                        continue;
                    }
                    let fid = field.ident.as_ref().expect("named field");
                    let fname = fa.rename.clone().unwrap_or_else(|| ident_name(fid));
                    out.push(field_tokens(&fname, &field.ty, &fa));
                }
                out
            }
            Fields::Unnamed(_) => {
                return Err(syn::Error::new(
                    v.span(),
                    "schemapb: a tagged enum's variants must be unit or struct variants",
                ))
            }
        };
        variants.push(quote! {
            (
                ::std::string::String::from(#name),
                ::schemapb::gen::schemapb::Schema {
                    fields: ::std::vec![#(#fields),*],
                    ..::std::default::Default::default()
                },
            )
        });
    }
    let decorate = decorate_tokens(attrs);
    Ok(quote! {
        fn field(name: &str) -> ::schemapb::gen::schemapb::schema::Field {
            ::schemapb::gen::schemapb::schema::Field {
                name: ::std::string::String::from(name),
                kind: ::std::option::Option::Some(
                    ::schemapb::gen::schemapb::schema::field::Kind::OneOf(
                        ::schemapb::gen::schemapb::schema::field::OneOf {
                            discriminator: ::std::string::String::from(#tag),
                            variants: [#(#variants),*].into_iter().collect(),
                        },
                    ),
                ),
                ..::std::default::Default::default()
            }
        }

        #decorate
    })
}

/// A unit enum's default is one of its variants (`default = Status::Open`).
fn enum_default(
    data: &DataEnum,
    attrs: &ContainerAttrs,
    ident: &Ident,
    generics: &syn::Generics,
) -> syn::Result<TokenStream2> {
    let mut arms = Vec::new();
    for v in &data.variants {
        let vid = &v.ident;
        let name = variant_name(v, attrs)?.unwrap_or_else(|| ident_name(vid));
        arms.push(quote! { #ident::#vid => #name });
    }
    let (impl_generics, ty_generics, where_clause) = generics.split_for_impl();
    Ok(quote! {
        impl #impl_generics ::schemapb::reflect::attr::HasDefault for #ident #ty_generics #where_clause {
            type Arg = Self;
            fn default(f: &mut ::schemapb::gen::schemapb::schema::Field, v: Self) {
                ::schemapb::reflect::attr::default_variant(f, match v { #(#arms),* });
            }
        }
    })
}

fn validate(input: &DeriveInput) -> syn::Result<TokenStream2> {
    let attrs = container_attrs(&input.attrs)?;
    if matches!(input.data, Data::Enum(_)) {
        return Err(syn::Error::new_spanned(
            &input.ident,
            "schemapb: derive Validate on a struct; an enum is a field (derive Reflect)",
        ));
    }
    let ident = &input.ident;
    let generics = add_bounds(&input.generics);
    let (impl_generics, ty_generics, where_clause) = generics.split_for_impl();
    let namespace = attrs.namespace.map_or_else(
        || quote! { ::std::env!("CARGO_PKG_NAME") },
        |n| quote! { #n },
    );
    let name = attrs.name.map_or_else(
        || {
            let s = ident.to_string();
            quote! { #s }
        },
        |n| quote! { #n },
    );
    let version = attrs
        .version
        .map_or_else(|| quote! { "" }, |v| quote! { #v });
    // A generic type serializes when its parameters do (serde's own bound).
    let mut where_clause = where_clause
        .cloned()
        .unwrap_or_else(|| syn::parse_quote!(where));
    where_clause.predicates.push(syn::parse_quote!(
        #ident #ty_generics: ::schemapb::__private::serde::Serialize
    ));
    Ok(quote! {
        impl #impl_generics ::schemapb::Validate for #ident #ty_generics #where_clause {
            fn schema_id() -> ::schemapb::gen::schemapb::SchemaIdentity {
                ::schemapb::gen::schemapb::SchemaIdentity {
                    namespace: ::std::string::String::from(#namespace),
                    name: ::std::string::String::from(#name),
                    version: ::std::string::String::from(#version),
                }
            }
        }
    })
}
