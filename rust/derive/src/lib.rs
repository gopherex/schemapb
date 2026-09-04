//! `#[derive(Reflect)]`: a Rust struct reflected into a schemapb Schema.
//!
//! The macro generates an impl of `schemapb::reflect::ReflectField`; the
//! runtime side (leaf/container impls, cycle detection, constraint
//! application) lives in the `schemapb` crate. Field attributes:
//!
//! ```text
//! #[schemapb(desc = "...", rename = "x", required, secret, skip,
//!            min = 1, max = 64, len = 4,
//!            gte = 0, gt = -10, lte = 100, lt = 10,
//!            pattern = "^[a-z-]+$", format = "email",
//!            oneof("fast", "safe"))]      // or oneof(1, 2, 3)
//! ```
//!
//! Nested types recurse through the trait — implementing `ReflectField`
//! for your own type IS the override mechanism (serde-style).

use proc_macro::TokenStream;
use proc_macro2::TokenStream as TokenStream2;
use quote::quote;
use syn::{parse_macro_input, Data, DeriveInput, Expr, Fields, Lit};

#[proc_macro_derive(Reflect, attributes(schemapb))]
pub fn derive_reflect(input: TokenStream) -> TokenStream {
    let input = parse_macro_input!(input as DeriveInput);
    expand(&input)
        .unwrap_or_else(syn::Error::into_compile_error)
        .into()
}

struct FieldAttrs {
    desc: Option<String>,
    rename: Option<String>,
    required: bool,
    secret: bool,
    skip: bool,
    oneof_str: Vec<String>,
    oneof_int: Vec<i64>,
    constraints: Vec<(String, String)>,
}

fn expand(input: &DeriveInput) -> syn::Result<TokenStream2> {
    let Data::Struct(data) = &input.data else {
        return Err(syn::Error::new_spanned(
            input,
            "#[derive(Reflect)] supports structs only",
        ));
    };

    let Fields::Named(named) = &data.fields else {
        return Err(syn::Error::new_spanned(
            input,
            "#[derive(Reflect)] requires named fields",
        ));
    };

    let mut field_exprs = Vec::new();

    for field in &named.named {
        let attrs = parse_attrs(field)?;
        if attrs.skip {
            continue;
        }

        let ident = field.ident.as_ref().expect("named field");
        let name = attrs.rename.clone().unwrap_or_else(|| ident.to_string());
        let ty = &field.ty;

        let base = if !attrs.oneof_str.is_empty() {
            let opts = &attrs.oneof_str;
            quote! { ::schemapb::reflect::apply::choice_str(#name, &[#(#opts),*]) }
        } else if !attrs.oneof_int.is_empty() {
            let opts = &attrs.oneof_int;
            quote! { ::schemapb::reflect::apply::choice_int(#name, &[#(#opts),*]) }
        } else {
            quote! { <#ty as ::schemapb::reflect::ReflectField>::field(#name) }
        };

        let mut setters = Vec::new();

        if attrs.oneof_str.is_empty() && attrs.oneof_int.is_empty() {
            for (key, raw) in &attrs.constraints {
                setters.push(quote! {
                    ::schemapb::reflect::apply::constraint(&mut f, #key, #raw);
                });
            }
        }

        if let Some(d) = &attrs.desc {
            setters.push(quote! { ::schemapb::reflect::apply::desc(&mut f, #d); });
        }

        if attrs.secret {
            setters.push(quote! { ::schemapb::reflect::apply::secret(&mut f); });
        }

        let required = if attrs.required {
            quote! { f.required = true; }
        } else {
            quote! { f.required = <#ty as ::schemapb::reflect::ReflectField>::REQUIRED; }
        };

        field_exprs.push(quote! {
            {
                let mut f = #base;
                #required
                #(#setters)*
                f
            }
        });
    }

    let ident = &input.ident;
    let (impl_generics, ty_generics, where_clause) = input.generics.split_for_impl();

    Ok(quote! {
        impl #impl_generics ::schemapb::reflect::ReflectField for #ident #ty_generics #where_clause {
            fn field(name: &str) -> ::schemapb::gen::schemapb::schema::Field {
                ::schemapb::reflect::visiting(
                    ::std::any::TypeId::of::<Self>(),
                    || ::schemapb::reflect::object_field(
                        name,
                        <Self as ::schemapb::reflect::ReflectField>::object_fields()
                            .expect("derived structs always have object fields"),
                    ),
                )
                .unwrap_or_else(|| ::schemapb::reflect::json_field(name))
            }

            fn object_fields() -> ::std::option::Option<
                ::std::vec::Vec<::schemapb::gen::schemapb::schema::Field>,
            > {
                ::std::option::Option::Some(::std::vec![#(#field_exprs),*])
            }
        }
    })
}

fn parse_attrs(field: &syn::Field) -> syn::Result<FieldAttrs> {
    let mut out = FieldAttrs {
        desc: None,
        rename: None,
        required: false,
        secret: false,
        skip: false,
        oneof_str: Vec::new(),
        oneof_int: Vec::new(),
        constraints: Vec::new(),
    };

    for attr in &field.attrs {
        if !attr.path().is_ident("schemapb") {
            continue;
        }

        attr.parse_nested_meta(|meta| {
            let key = meta
                .path
                .get_ident()
                .map(ToString::to_string)
                .unwrap_or_default();

            match key.as_str() {
                "required" => out.required = true,
                "secret" => out.secret = true,
                "skip" => out.skip = true,
                "desc" => out.desc = Some(lit_str(&meta)?),
                "rename" => out.rename = Some(lit_str(&meta)?),
                "pattern" | "format" => {
                    let v = lit_str(&meta)?;
                    out.constraints.push((key, v));
                }
                "min" | "max" | "len" | "gte" | "gt" | "lte" | "lt" => {
                    let v = lit_text(&meta)?;
                    out.constraints.push((key, v));
                }
                "oneof" => {
                    let content;
                    syn::parenthesized!(content in meta.input);
                    let lits = content
                        .parse_terminated(<Lit as syn::parse::Parse>::parse, syn::Token![,])?;
                    for lit in lits {
                        match lit {
                            Lit::Str(s) => out.oneof_str.push(s.value()),
                            Lit::Int(i) => out.oneof_int.push(i.base10_parse()?),
                            other => {
                                return Err(syn::Error::new_spanned(
                                    other,
                                    "oneof options must be string or integer literals",
                                ))
                            }
                        }
                    }
                    if !out.oneof_str.is_empty() && !out.oneof_int.is_empty() {
                        return Err(meta.error("oneof options must be all-string or all-integer"));
                    }
                }
                other => return Err(meta.error(format!("unknown schemapb attribute {other:?}"))),
            }

            Ok(())
        })?;
    }

    Ok(out)
}

fn lit_str(meta: &syn::meta::ParseNestedMeta) -> syn::Result<String> {
    let expr: Expr = meta.value()?.parse()?;
    if let Expr::Lit(l) = &expr {
        if let Lit::Str(s) = &l.lit {
            return Ok(s.value());
        }
    }
    Err(meta.error("expected a string literal"))
}

/// The literal's textual form: numbers keep their exact spelling and are
/// parsed against the field's own kind at reflection time.
fn lit_text(meta: &syn::meta::ParseNestedMeta) -> syn::Result<String> {
    let expr: Expr = meta.value()?.parse()?;
    match &expr {
        Expr::Lit(l) => match &l.lit {
            Lit::Int(i) => Ok(i.base10_digits().to_owned()),
            Lit::Float(f) => Ok(f.base10_digits().to_owned()),
            Lit::Str(s) => Ok(s.value()),
            _ => Err(meta.error("expected a numeric literal")),
        },
        Expr::Unary(syn::ExprUnary {
            op: syn::UnOp::Neg(_),
            expr,
            ..
        }) => {
            if let Expr::Lit(l) = expr.as_ref() {
                match &l.lit {
                    Lit::Int(i) => Ok(format!("-{}", i.base10_digits())),
                    Lit::Float(f) => Ok(format!("-{}", f.base10_digits())),
                    _ => Err(meta.error("expected a numeric literal")),
                }
            } else {
                Err(meta.error("expected a numeric literal"))
            }
        }
        _ => Err(meta.error("expected a literal")),
    }
}
