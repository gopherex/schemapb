// @generated
impl serde::Serialize for Baked {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.schema.is_some() {
            len += 1;
        }
        if self.values.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Baked", len)?;
        if let Some(v) = self.schema.as_ref() {
            struct_ser.serialize_field("schema", v)?;
        }
        if let Some(v) = self.values.as_ref() {
            struct_ser.serialize_field("values", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Baked {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "schema",
            "values",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Schema,
            Values,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "schema" => Ok(GeneratedField::Schema),
                            "values" => Ok(GeneratedField::Values),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Baked;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Baked")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Baked, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut schema__ = None;
                let mut values__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Schema => {
                            if schema__.is_some() {
                                return Err(serde::de::Error::duplicate_field("schema"));
                            }
                            schema__ = map_.next_value()?;
                        }
                        GeneratedField::Values => {
                            if values__.is_some() {
                                return Err(serde::de::Error::duplicate_field("values"));
                            }
                            values__ = map_.next_value()?;
                        }
                    }
                }
                Ok(Baked {
                    schema: schema__,
                    values: values__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Baked", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ErrorCode {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "ERROR_CODE_UNSPECIFIED",
            Self::TypeMismatch => "ERROR_CODE_TYPE_MISMATCH",
            Self::RequiredMissing => "ERROR_CODE_REQUIRED_MISSING",
            Self::NotNullable => "ERROR_CODE_NOT_NULLABLE",
            Self::UnknownField => "ERROR_CODE_UNKNOWN_FIELD",
            Self::ImmutableModified => "ERROR_CODE_IMMUTABLE_MODIFIED",
            Self::ConstMismatch => "ERROR_CODE_CONST_MISMATCH",
            Self::GtViolated => "ERROR_CODE_GT_VIOLATED",
            Self::GteViolated => "ERROR_CODE_GTE_VIOLATED",
            Self::LtViolated => "ERROR_CODE_LT_VIOLATED",
            Self::LteViolated => "ERROR_CODE_LTE_VIOLATED",
            Self::NotInAllowedSet => "ERROR_CODE_NOT_IN_ALLOWED_SET",
            Self::InForbiddenSet => "ERROR_CODE_IN_FORBIDDEN_SET",
            Self::MultipleOfViolated => "ERROR_CODE_MULTIPLE_OF_VIOLATED",
            Self::LenMismatch => "ERROR_CODE_LEN_MISMATCH",
            Self::MinLenViolated => "ERROR_CODE_MIN_LEN_VIOLATED",
            Self::MaxLenViolated => "ERROR_CODE_MAX_LEN_VIOLATED",
            Self::PatternMismatch => "ERROR_CODE_PATTERN_MISMATCH",
            Self::FormatMismatch => "ERROR_CODE_FORMAT_MISMATCH",
            Self::PrefixMismatch => "ERROR_CODE_PREFIX_MISMATCH",
            Self::SuffixMismatch => "ERROR_CODE_SUFFIX_MISMATCH",
            Self::UnsupportedFormat => "ERROR_CODE_UNSUPPORTED_FORMAT",
            Self::ChoiceNotAllowed => "ERROR_CODE_CHOICE_NOT_ALLOWED",
            Self::MinItemsViolated => "ERROR_CODE_MIN_ITEMS_VIOLATED",
            Self::MaxItemsViolated => "ERROR_CODE_MAX_ITEMS_VIOLATED",
            Self::NotUnique => "ERROR_CODE_NOT_UNIQUE",
            Self::ListCountMismatch => "ERROR_CODE_LIST_COUNT_MISMATCH",
            Self::MinEntriesViolated => "ERROR_CODE_MIN_ENTRIES_VIOLATED",
            Self::MaxEntriesViolated => "ERROR_CODE_MAX_ENTRIES_VIOLATED",
            Self::MinPropertiesViolated => "ERROR_CODE_MIN_PROPERTIES_VIOLATED",
            Self::MaxPropertiesViolated => "ERROR_CODE_MAX_PROPERTIES_VIOLATED",
            Self::DiscriminatorMissing => "ERROR_CODE_DISCRIMINATOR_MISSING",
            Self::UnknownVariant => "ERROR_CODE_UNKNOWN_VARIANT",
            Self::UnknownRef => "ERROR_CODE_UNKNOWN_REF",
            Self::RuleViolated => "ERROR_CODE_RULE_VIOLATED",
            Self::ExprError => "ERROR_CODE_EXPR_ERROR",
            Self::InvalidSchema => "ERROR_CODE_INVALID_SCHEMA",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for ErrorCode {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "ERROR_CODE_UNSPECIFIED",
            "ERROR_CODE_TYPE_MISMATCH",
            "ERROR_CODE_REQUIRED_MISSING",
            "ERROR_CODE_NOT_NULLABLE",
            "ERROR_CODE_UNKNOWN_FIELD",
            "ERROR_CODE_IMMUTABLE_MODIFIED",
            "ERROR_CODE_CONST_MISMATCH",
            "ERROR_CODE_GT_VIOLATED",
            "ERROR_CODE_GTE_VIOLATED",
            "ERROR_CODE_LT_VIOLATED",
            "ERROR_CODE_LTE_VIOLATED",
            "ERROR_CODE_NOT_IN_ALLOWED_SET",
            "ERROR_CODE_IN_FORBIDDEN_SET",
            "ERROR_CODE_MULTIPLE_OF_VIOLATED",
            "ERROR_CODE_LEN_MISMATCH",
            "ERROR_CODE_MIN_LEN_VIOLATED",
            "ERROR_CODE_MAX_LEN_VIOLATED",
            "ERROR_CODE_PATTERN_MISMATCH",
            "ERROR_CODE_FORMAT_MISMATCH",
            "ERROR_CODE_PREFIX_MISMATCH",
            "ERROR_CODE_SUFFIX_MISMATCH",
            "ERROR_CODE_UNSUPPORTED_FORMAT",
            "ERROR_CODE_CHOICE_NOT_ALLOWED",
            "ERROR_CODE_MIN_ITEMS_VIOLATED",
            "ERROR_CODE_MAX_ITEMS_VIOLATED",
            "ERROR_CODE_NOT_UNIQUE",
            "ERROR_CODE_LIST_COUNT_MISMATCH",
            "ERROR_CODE_MIN_ENTRIES_VIOLATED",
            "ERROR_CODE_MAX_ENTRIES_VIOLATED",
            "ERROR_CODE_MIN_PROPERTIES_VIOLATED",
            "ERROR_CODE_MAX_PROPERTIES_VIOLATED",
            "ERROR_CODE_DISCRIMINATOR_MISSING",
            "ERROR_CODE_UNKNOWN_VARIANT",
            "ERROR_CODE_UNKNOWN_REF",
            "ERROR_CODE_RULE_VIOLATED",
            "ERROR_CODE_EXPR_ERROR",
            "ERROR_CODE_INVALID_SCHEMA",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ErrorCode;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "ERROR_CODE_UNSPECIFIED" => Ok(ErrorCode::Unspecified),
                    "ERROR_CODE_TYPE_MISMATCH" => Ok(ErrorCode::TypeMismatch),
                    "ERROR_CODE_REQUIRED_MISSING" => Ok(ErrorCode::RequiredMissing),
                    "ERROR_CODE_NOT_NULLABLE" => Ok(ErrorCode::NotNullable),
                    "ERROR_CODE_UNKNOWN_FIELD" => Ok(ErrorCode::UnknownField),
                    "ERROR_CODE_IMMUTABLE_MODIFIED" => Ok(ErrorCode::ImmutableModified),
                    "ERROR_CODE_CONST_MISMATCH" => Ok(ErrorCode::ConstMismatch),
                    "ERROR_CODE_GT_VIOLATED" => Ok(ErrorCode::GtViolated),
                    "ERROR_CODE_GTE_VIOLATED" => Ok(ErrorCode::GteViolated),
                    "ERROR_CODE_LT_VIOLATED" => Ok(ErrorCode::LtViolated),
                    "ERROR_CODE_LTE_VIOLATED" => Ok(ErrorCode::LteViolated),
                    "ERROR_CODE_NOT_IN_ALLOWED_SET" => Ok(ErrorCode::NotInAllowedSet),
                    "ERROR_CODE_IN_FORBIDDEN_SET" => Ok(ErrorCode::InForbiddenSet),
                    "ERROR_CODE_MULTIPLE_OF_VIOLATED" => Ok(ErrorCode::MultipleOfViolated),
                    "ERROR_CODE_LEN_MISMATCH" => Ok(ErrorCode::LenMismatch),
                    "ERROR_CODE_MIN_LEN_VIOLATED" => Ok(ErrorCode::MinLenViolated),
                    "ERROR_CODE_MAX_LEN_VIOLATED" => Ok(ErrorCode::MaxLenViolated),
                    "ERROR_CODE_PATTERN_MISMATCH" => Ok(ErrorCode::PatternMismatch),
                    "ERROR_CODE_FORMAT_MISMATCH" => Ok(ErrorCode::FormatMismatch),
                    "ERROR_CODE_PREFIX_MISMATCH" => Ok(ErrorCode::PrefixMismatch),
                    "ERROR_CODE_SUFFIX_MISMATCH" => Ok(ErrorCode::SuffixMismatch),
                    "ERROR_CODE_UNSUPPORTED_FORMAT" => Ok(ErrorCode::UnsupportedFormat),
                    "ERROR_CODE_CHOICE_NOT_ALLOWED" => Ok(ErrorCode::ChoiceNotAllowed),
                    "ERROR_CODE_MIN_ITEMS_VIOLATED" => Ok(ErrorCode::MinItemsViolated),
                    "ERROR_CODE_MAX_ITEMS_VIOLATED" => Ok(ErrorCode::MaxItemsViolated),
                    "ERROR_CODE_NOT_UNIQUE" => Ok(ErrorCode::NotUnique),
                    "ERROR_CODE_LIST_COUNT_MISMATCH" => Ok(ErrorCode::ListCountMismatch),
                    "ERROR_CODE_MIN_ENTRIES_VIOLATED" => Ok(ErrorCode::MinEntriesViolated),
                    "ERROR_CODE_MAX_ENTRIES_VIOLATED" => Ok(ErrorCode::MaxEntriesViolated),
                    "ERROR_CODE_MIN_PROPERTIES_VIOLATED" => Ok(ErrorCode::MinPropertiesViolated),
                    "ERROR_CODE_MAX_PROPERTIES_VIOLATED" => Ok(ErrorCode::MaxPropertiesViolated),
                    "ERROR_CODE_DISCRIMINATOR_MISSING" => Ok(ErrorCode::DiscriminatorMissing),
                    "ERROR_CODE_UNKNOWN_VARIANT" => Ok(ErrorCode::UnknownVariant),
                    "ERROR_CODE_UNKNOWN_REF" => Ok(ErrorCode::UnknownRef),
                    "ERROR_CODE_RULE_VIOLATED" => Ok(ErrorCode::RuleViolated),
                    "ERROR_CODE_EXPR_ERROR" => Ok(ErrorCode::ExprError),
                    "ERROR_CODE_INVALID_SCHEMA" => Ok(ErrorCode::InvalidSchema),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for Filled {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.schema.is_some() {
            len += 1;
        }
        if self.values.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Filled", len)?;
        if let Some(v) = self.schema.as_ref() {
            struct_ser.serialize_field("schema", v)?;
        }
        if let Some(v) = self.values.as_ref() {
            struct_ser.serialize_field("values", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Filled {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "schema",
            "values",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Schema,
            Values,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "schema" => Ok(GeneratedField::Schema),
                            "values" => Ok(GeneratedField::Values),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Filled;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Filled")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Filled, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut schema__ = None;
                let mut values__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Schema => {
                            if schema__.is_some() {
                                return Err(serde::de::Error::duplicate_field("schema"));
                            }
                            schema__ = map_.next_value()?;
                        }
                        GeneratedField::Values => {
                            if values__.is_some() {
                                return Err(serde::de::Error::duplicate_field("values"));
                            }
                            values__ = map_.next_value()?;
                        }
                    }
                }
                Ok(Filled {
                    schema: schema__,
                    values: values__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Filled", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListValue {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.items.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.ListValue", len)?;
        if !self.items.is_empty() {
            struct_ser.serialize_field("items", &self.items)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListValue {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "items",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Items,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "items" => Ok(GeneratedField::Items),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListValue;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.ListValue")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListValue, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut items__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Items => {
                            if items__.is_some() {
                                return Err(serde::de::Error::duplicate_field("items"));
                            }
                            items__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListValue {
                    items: items__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.ListValue", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for NullValue {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::NullValue => "NULL_VALUE",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for NullValue {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "NULL_VALUE",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = NullValue;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "NULL_VALUE" => Ok(NullValue::NullValue),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for Schema {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.id.is_some() {
            len += 1;
        }
        if self.description.is_some() {
            len += 1;
        }
        if !self.fields.is_empty() {
            len += 1;
        }
        if !self.rules.is_empty() {
            len += 1;
        }
        if self.strict {
            len += 1;
        }
        if self.min_properties.is_some() {
            len += 1;
        }
        if self.max_properties.is_some() {
            len += 1;
        }
        if self.coerce {
            len += 1;
        }
        if !self.defs.is_empty() {
            len += 1;
        }
        if !self.templates.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema", len)?;
        if let Some(v) = self.id.as_ref() {
            struct_ser.serialize_field("id", v)?;
        }
        if let Some(v) = self.description.as_ref() {
            struct_ser.serialize_field("description", v)?;
        }
        if !self.fields.is_empty() {
            struct_ser.serialize_field("fields", &self.fields)?;
        }
        if !self.rules.is_empty() {
            struct_ser.serialize_field("rules", &self.rules)?;
        }
        if self.strict {
            struct_ser.serialize_field("strict", &self.strict)?;
        }
        if let Some(v) = self.min_properties.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("minProperties", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.max_properties.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("maxProperties", ToString::to_string(&v).as_str())?;
        }
        if self.coerce {
            struct_ser.serialize_field("coerce", &self.coerce)?;
        }
        if !self.defs.is_empty() {
            struct_ser.serialize_field("defs", &self.defs)?;
        }
        if !self.templates.is_empty() {
            struct_ser.serialize_field("templates", &self.templates)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Schema {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "description",
            "fields",
            "rules",
            "strict",
            "min_properties",
            "minProperties",
            "max_properties",
            "maxProperties",
            "coerce",
            "defs",
            "templates",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            Description,
            Fields,
            Rules,
            Strict,
            MinProperties,
            MaxProperties,
            Coerce,
            Defs,
            Templates,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "id" => Ok(GeneratedField::Id),
                            "description" => Ok(GeneratedField::Description),
                            "fields" => Ok(GeneratedField::Fields),
                            "rules" => Ok(GeneratedField::Rules),
                            "strict" => Ok(GeneratedField::Strict),
                            "minProperties" | "min_properties" => Ok(GeneratedField::MinProperties),
                            "maxProperties" | "max_properties" => Ok(GeneratedField::MaxProperties),
                            "coerce" => Ok(GeneratedField::Coerce),
                            "defs" => Ok(GeneratedField::Defs),
                            "templates" => Ok(GeneratedField::Templates),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Schema;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Schema, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut description__ = None;
                let mut fields__ = None;
                let mut rules__ = None;
                let mut strict__ = None;
                let mut min_properties__ = None;
                let mut max_properties__ = None;
                let mut coerce__ = None;
                let mut defs__ = None;
                let mut templates__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = map_.next_value()?;
                        }
                        GeneratedField::Description => {
                            if description__.is_some() {
                                return Err(serde::de::Error::duplicate_field("description"));
                            }
                            description__ = map_.next_value()?;
                        }
                        GeneratedField::Fields => {
                            if fields__.is_some() {
                                return Err(serde::de::Error::duplicate_field("fields"));
                            }
                            fields__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Rules => {
                            if rules__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rules"));
                            }
                            rules__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Strict => {
                            if strict__.is_some() {
                                return Err(serde::de::Error::duplicate_field("strict"));
                            }
                            strict__ = Some(map_.next_value()?);
                        }
                        GeneratedField::MinProperties => {
                            if min_properties__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minProperties"));
                            }
                            min_properties__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::MaxProperties => {
                            if max_properties__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxProperties"));
                            }
                            max_properties__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Coerce => {
                            if coerce__.is_some() {
                                return Err(serde::de::Error::duplicate_field("coerce"));
                            }
                            coerce__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Defs => {
                            if defs__.is_some() {
                                return Err(serde::de::Error::duplicate_field("defs"));
                            }
                            defs__ = Some(
                                map_.next_value::<std::collections::HashMap<_, _>>()?
                            );
                        }
                        GeneratedField::Templates => {
                            if templates__.is_some() {
                                return Err(serde::de::Error::duplicate_field("templates"));
                            }
                            templates__ = Some(
                                map_.next_value::<std::collections::HashMap<_, _>>()?
                            );
                        }
                    }
                }
                Ok(Schema {
                    id: id__,
                    description: description__,
                    fields: fields__.unwrap_or_default(),
                    rules: rules__.unwrap_or_default(),
                    strict: strict__.unwrap_or_default(),
                    min_properties: min_properties__,
                    max_properties: max_properties__,
                    coerce: coerce__.unwrap_or_default(),
                    defs: defs__.unwrap_or_default(),
                    templates: templates__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::Field {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.name.is_empty() {
            len += 1;
        }
        if self.description.is_some() {
            len += 1;
        }
        if self.nullable {
            len += 1;
        }
        if self.required {
            len += 1;
        }
        if !self.rules.is_empty() {
            len += 1;
        }
        if self.immutable {
            len += 1;
        }
        if self.group.is_some() {
            len += 1;
        }
        if self.unit.is_some() {
            len += 1;
        }
        if self.title.is_some() {
            len += 1;
        }
        if self.deprecated {
            len += 1;
        }
        if !self.examples.is_empty() {
            len += 1;
        }
        if self.secret {
            len += 1;
        }
        if self.normalize.is_some() {
            len += 1;
        }
        if self.when.is_some() {
            len += 1;
        }
        if self.kind.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field", len)?;
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if let Some(v) = self.description.as_ref() {
            struct_ser.serialize_field("description", v)?;
        }
        if self.nullable {
            struct_ser.serialize_field("nullable", &self.nullable)?;
        }
        if self.required {
            struct_ser.serialize_field("required", &self.required)?;
        }
        if !self.rules.is_empty() {
            struct_ser.serialize_field("rules", &self.rules)?;
        }
        if self.immutable {
            struct_ser.serialize_field("immutable", &self.immutable)?;
        }
        if let Some(v) = self.group.as_ref() {
            struct_ser.serialize_field("group", v)?;
        }
        if let Some(v) = self.unit.as_ref() {
            struct_ser.serialize_field("unit", v)?;
        }
        if let Some(v) = self.title.as_ref() {
            struct_ser.serialize_field("title", v)?;
        }
        if self.deprecated {
            struct_ser.serialize_field("deprecated", &self.deprecated)?;
        }
        if !self.examples.is_empty() {
            struct_ser.serialize_field("examples", &self.examples)?;
        }
        if self.secret {
            struct_ser.serialize_field("secret", &self.secret)?;
        }
        if let Some(v) = self.normalize.as_ref() {
            struct_ser.serialize_field("normalize", v)?;
        }
        if let Some(v) = self.when.as_ref() {
            struct_ser.serialize_field("when", v)?;
        }
        if let Some(v) = self.kind.as_ref() {
            match v {
                schema::field::Kind::Float(v) => {
                    struct_ser.serialize_field("float", v)?;
                }
                schema::field::Kind::Double(v) => {
                    struct_ser.serialize_field("double", v)?;
                }
                schema::field::Kind::Int32(v) => {
                    struct_ser.serialize_field("int32", v)?;
                }
                schema::field::Kind::Int64(v) => {
                    struct_ser.serialize_field("int64", v)?;
                }
                schema::field::Kind::Uint32(v) => {
                    struct_ser.serialize_field("uint32", v)?;
                }
                schema::field::Kind::Uint64(v) => {
                    struct_ser.serialize_field("uint64", v)?;
                }
                schema::field::Kind::Bool(v) => {
                    struct_ser.serialize_field("bool", v)?;
                }
                schema::field::Kind::String(v) => {
                    struct_ser.serialize_field("string", v)?;
                }
                schema::field::Kind::Choice(v) => {
                    struct_ser.serialize_field("choice", v)?;
                }
                schema::field::Kind::Duration(v) => {
                    struct_ser.serialize_field("duration", v)?;
                }
                schema::field::Kind::Timestamp(v) => {
                    struct_ser.serialize_field("timestamp", v)?;
                }
                schema::field::Kind::List(v) => {
                    struct_ser.serialize_field("list", v)?;
                }
                schema::field::Kind::Object(v) => {
                    struct_ser.serialize_field("object", v)?;
                }
                schema::field::Kind::Computed(v) => {
                    struct_ser.serialize_field("computed", v)?;
                }
                schema::field::Kind::OneOf(v) => {
                    struct_ser.serialize_field("oneOf", v)?;
                }
                schema::field::Kind::Ref(v) => {
                    struct_ser.serialize_field("ref", v)?;
                }
                schema::field::Kind::Map(v) => {
                    struct_ser.serialize_field("map", v)?;
                }
                schema::field::Kind::Bytes(v) => {
                    struct_ser.serialize_field("bytes", v)?;
                }
                schema::field::Kind::Json(v) => {
                    struct_ser.serialize_field("json", v)?;
                }
            }
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::Field {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "name",
            "description",
            "nullable",
            "required",
            "rules",
            "immutable",
            "group",
            "unit",
            "title",
            "deprecated",
            "examples",
            "secret",
            "normalize",
            "when",
            "float",
            "double",
            "int32",
            "int64",
            "uint32",
            "uint64",
            "bool",
            "string",
            "choice",
            "duration",
            "timestamp",
            "list",
            "object",
            "computed",
            "one_of",
            "oneOf",
            "ref",
            "map",
            "bytes",
            "json",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Name,
            Description,
            Nullable,
            Required,
            Rules,
            Immutable,
            Group,
            Unit,
            Title,
            Deprecated,
            Examples,
            Secret,
            Normalize,
            When,
            Float,
            Double,
            Int32,
            Int64,
            Uint32,
            Uint64,
            Bool,
            String,
            Choice,
            Duration,
            Timestamp,
            List,
            Object,
            Computed,
            OneOf,
            Ref,
            Map,
            Bytes,
            Json,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "name" => Ok(GeneratedField::Name),
                            "description" => Ok(GeneratedField::Description),
                            "nullable" => Ok(GeneratedField::Nullable),
                            "required" => Ok(GeneratedField::Required),
                            "rules" => Ok(GeneratedField::Rules),
                            "immutable" => Ok(GeneratedField::Immutable),
                            "group" => Ok(GeneratedField::Group),
                            "unit" => Ok(GeneratedField::Unit),
                            "title" => Ok(GeneratedField::Title),
                            "deprecated" => Ok(GeneratedField::Deprecated),
                            "examples" => Ok(GeneratedField::Examples),
                            "secret" => Ok(GeneratedField::Secret),
                            "normalize" => Ok(GeneratedField::Normalize),
                            "when" => Ok(GeneratedField::When),
                            "float" => Ok(GeneratedField::Float),
                            "double" => Ok(GeneratedField::Double),
                            "int32" => Ok(GeneratedField::Int32),
                            "int64" => Ok(GeneratedField::Int64),
                            "uint32" => Ok(GeneratedField::Uint32),
                            "uint64" => Ok(GeneratedField::Uint64),
                            "bool" => Ok(GeneratedField::Bool),
                            "string" => Ok(GeneratedField::String),
                            "choice" => Ok(GeneratedField::Choice),
                            "duration" => Ok(GeneratedField::Duration),
                            "timestamp" => Ok(GeneratedField::Timestamp),
                            "list" => Ok(GeneratedField::List),
                            "object" => Ok(GeneratedField::Object),
                            "computed" => Ok(GeneratedField::Computed),
                            "oneOf" | "one_of" => Ok(GeneratedField::OneOf),
                            "ref" => Ok(GeneratedField::Ref),
                            "map" => Ok(GeneratedField::Map),
                            "bytes" => Ok(GeneratedField::Bytes),
                            "json" => Ok(GeneratedField::Json),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::Field;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::Field, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut name__ = None;
                let mut description__ = None;
                let mut nullable__ = None;
                let mut required__ = None;
                let mut rules__ = None;
                let mut immutable__ = None;
                let mut group__ = None;
                let mut unit__ = None;
                let mut title__ = None;
                let mut deprecated__ = None;
                let mut examples__ = None;
                let mut secret__ = None;
                let mut normalize__ = None;
                let mut when__ = None;
                let mut kind__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Description => {
                            if description__.is_some() {
                                return Err(serde::de::Error::duplicate_field("description"));
                            }
                            description__ = map_.next_value()?;
                        }
                        GeneratedField::Nullable => {
                            if nullable__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nullable"));
                            }
                            nullable__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Required => {
                            if required__.is_some() {
                                return Err(serde::de::Error::duplicate_field("required"));
                            }
                            required__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Rules => {
                            if rules__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rules"));
                            }
                            rules__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Immutable => {
                            if immutable__.is_some() {
                                return Err(serde::de::Error::duplicate_field("immutable"));
                            }
                            immutable__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Group => {
                            if group__.is_some() {
                                return Err(serde::de::Error::duplicate_field("group"));
                            }
                            group__ = map_.next_value()?;
                        }
                        GeneratedField::Unit => {
                            if unit__.is_some() {
                                return Err(serde::de::Error::duplicate_field("unit"));
                            }
                            unit__ = map_.next_value()?;
                        }
                        GeneratedField::Title => {
                            if title__.is_some() {
                                return Err(serde::de::Error::duplicate_field("title"));
                            }
                            title__ = map_.next_value()?;
                        }
                        GeneratedField::Deprecated => {
                            if deprecated__.is_some() {
                                return Err(serde::de::Error::duplicate_field("deprecated"));
                            }
                            deprecated__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Examples => {
                            if examples__.is_some() {
                                return Err(serde::de::Error::duplicate_field("examples"));
                            }
                            examples__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Secret => {
                            if secret__.is_some() {
                                return Err(serde::de::Error::duplicate_field("secret"));
                            }
                            secret__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Normalize => {
                            if normalize__.is_some() {
                                return Err(serde::de::Error::duplicate_field("normalize"));
                            }
                            normalize__ = map_.next_value()?;
                        }
                        GeneratedField::When => {
                            if when__.is_some() {
                                return Err(serde::de::Error::duplicate_field("when"));
                            }
                            when__ = map_.next_value()?;
                        }
                        GeneratedField::Float => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("float"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Float)
;
                        }
                        GeneratedField::Double => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("double"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Double)
;
                        }
                        GeneratedField::Int32 => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("int32"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Int32)
;
                        }
                        GeneratedField::Int64 => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("int64"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Int64)
;
                        }
                        GeneratedField::Uint32 => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("uint32"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Uint32)
;
                        }
                        GeneratedField::Uint64 => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("uint64"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Uint64)
;
                        }
                        GeneratedField::Bool => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bool"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Bool)
;
                        }
                        GeneratedField::String => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("string"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::String)
;
                        }
                        GeneratedField::Choice => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("choice"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Choice)
;
                        }
                        GeneratedField::Duration => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("duration"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Duration)
;
                        }
                        GeneratedField::Timestamp => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timestamp"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Timestamp)
;
                        }
                        GeneratedField::List => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("list"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::List)
;
                        }
                        GeneratedField::Object => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("object"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Object)
;
                        }
                        GeneratedField::Computed => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("computed"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Computed)
;
                        }
                        GeneratedField::OneOf => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("oneOf"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::OneOf)
;
                        }
                        GeneratedField::Ref => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ref"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Ref)
;
                        }
                        GeneratedField::Map => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("map"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Map)
;
                        }
                        GeneratedField::Bytes => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bytes"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Bytes)
;
                        }
                        GeneratedField::Json => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("json"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::Kind::Json)
;
                        }
                    }
                }
                Ok(schema::Field {
                    name: name__.unwrap_or_default(),
                    description: description__,
                    nullable: nullable__.unwrap_or_default(),
                    required: required__.unwrap_or_default(),
                    rules: rules__.unwrap_or_default(),
                    immutable: immutable__.unwrap_or_default(),
                    group: group__,
                    unit: unit__,
                    title: title__,
                    deprecated: deprecated__.unwrap_or_default(),
                    examples: examples__.unwrap_or_default(),
                    secret: secret__.unwrap_or_default(),
                    normalize: normalize__,
                    when: when__,
                    kind: kind__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Bool {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Bool", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.r#const.as_ref() {
            struct_ser.serialize_field("const", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Bool {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Bool;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Bool")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Bool, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = map_.next_value()?;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::Bool {
                    default: default__,
                    r#const: r#const__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Bool", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Bytes {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.len.is_some() {
            len += 1;
        }
        if self.min_len.is_some() {
            len += 1;
        }
        if self.max_len.is_some() {
            len += 1;
        }
        if self.prefix.is_some() {
            len += 1;
        }
        if self.suffix.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Bytes", len)?;
        if let Some(v) = self.default.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("default", pbjson::private::base64::encode(&v).as_str())?;
        }
        if let Some(v) = self.r#const.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("const", pbjson::private::base64::encode(&v).as_str())?;
        }
        if let Some(v) = self.len.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("len", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.min_len.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("minLen", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.max_len.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("maxLen", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.prefix.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("prefix", pbjson::private::base64::encode(&v).as_str())?;
        }
        if let Some(v) = self.suffix.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("suffix", pbjson::private::base64::encode(&v).as_str())?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in.iter().map(pbjson::private::base64::encode).collect::<Vec<_>>())?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in.iter().map(pbjson::private::base64::encode).collect::<Vec<_>>())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Bytes {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "len",
            "min_len",
            "minLen",
            "max_len",
            "maxLen",
            "prefix",
            "suffix",
            "in",
            "not_in",
            "notIn",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Len,
            MinLen,
            MaxLen,
            Prefix,
            Suffix,
            In,
            NotIn,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "len" => Ok(GeneratedField::Len),
                            "minLen" | "min_len" => Ok(GeneratedField::MinLen),
                            "maxLen" | "max_len" => Ok(GeneratedField::MaxLen),
                            "prefix" => Ok(GeneratedField::Prefix),
                            "suffix" => Ok(GeneratedField::Suffix),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Bytes;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Bytes")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Bytes, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut len__ = None;
                let mut min_len__ = None;
                let mut max_len__ = None;
                let mut prefix__ = None;
                let mut suffix__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::BytesDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::BytesDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Len => {
                            if len__.is_some() {
                                return Err(serde::de::Error::duplicate_field("len"));
                            }
                            len__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::MinLen => {
                            if min_len__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minLen"));
                            }
                            min_len__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::MaxLen => {
                            if max_len__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxLen"));
                            }
                            max_len__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Prefix => {
                            if prefix__.is_some() {
                                return Err(serde::de::Error::duplicate_field("prefix"));
                            }
                            prefix__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::BytesDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Suffix => {
                            if suffix__.is_some() {
                                return Err(serde::de::Error::duplicate_field("suffix"));
                            }
                            suffix__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::BytesDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::BytesDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::BytesDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                    }
                }
                Ok(schema::field::Bytes {
                    default: default__,
                    r#const: r#const__,
                    len: len__,
                    min_len: min_len__,
                    max_len: max_len__,
                    prefix: prefix__,
                    suffix: suffix__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Bytes", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Choice {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.options.is_empty() {
            len += 1;
        }
        if self.default.is_some() {
            len += 1;
        }
        if self.open {
            len += 1;
        }
        if self.options_expr.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Choice", len)?;
        if !self.options.is_empty() {
            struct_ser.serialize_field("options", &self.options)?;
        }
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if self.open {
            struct_ser.serialize_field("open", &self.open)?;
        }
        if let Some(v) = self.options_expr.as_ref() {
            struct_ser.serialize_field("optionsExpr", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Choice {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "options",
            "default",
            "open",
            "options_expr",
            "optionsExpr",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Options,
            Default,
            Open,
            OptionsExpr,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "options" => Ok(GeneratedField::Options),
                            "default" => Ok(GeneratedField::Default),
                            "open" => Ok(GeneratedField::Open),
                            "optionsExpr" | "options_expr" => Ok(GeneratedField::OptionsExpr),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Choice;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Choice")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Choice, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut options__ = None;
                let mut default__ = None;
                let mut open__ = None;
                let mut options_expr__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Options => {
                            if options__.is_some() {
                                return Err(serde::de::Error::duplicate_field("options"));
                            }
                            options__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = map_.next_value()?;
                        }
                        GeneratedField::Open => {
                            if open__.is_some() {
                                return Err(serde::de::Error::duplicate_field("open"));
                            }
                            open__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OptionsExpr => {
                            if options_expr__.is_some() {
                                return Err(serde::de::Error::duplicate_field("optionsExpr"));
                            }
                            options_expr__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::Choice {
                    options: options__.unwrap_or_default(),
                    default: default__,
                    open: open__.unwrap_or_default(),
                    options_expr: options_expr__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Choice", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::choice::Option {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.value.is_some() {
            len += 1;
        }
        if !self.label.is_empty() {
            len += 1;
        }
        if !self.description.is_empty() {
            len += 1;
        }
        if self.deprecated {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Choice.Option", len)?;
        if let Some(v) = self.value.as_ref() {
            struct_ser.serialize_field("value", v)?;
        }
        if !self.label.is_empty() {
            struct_ser.serialize_field("label", &self.label)?;
        }
        if !self.description.is_empty() {
            struct_ser.serialize_field("description", &self.description)?;
        }
        if self.deprecated {
            struct_ser.serialize_field("deprecated", &self.deprecated)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::choice::Option {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "value",
            "label",
            "description",
            "deprecated",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Value,
            Label,
            Description,
            Deprecated,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "value" => Ok(GeneratedField::Value),
                            "label" => Ok(GeneratedField::Label),
                            "description" => Ok(GeneratedField::Description),
                            "deprecated" => Ok(GeneratedField::Deprecated),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::choice::Option;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Choice.Option")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::choice::Option, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut value__ = None;
                let mut label__ = None;
                let mut description__ = None;
                let mut deprecated__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Value => {
                            if value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("value"));
                            }
                            value__ = map_.next_value()?;
                        }
                        GeneratedField::Label => {
                            if label__.is_some() {
                                return Err(serde::de::Error::duplicate_field("label"));
                            }
                            label__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Description => {
                            if description__.is_some() {
                                return Err(serde::de::Error::duplicate_field("description"));
                            }
                            description__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Deprecated => {
                            if deprecated__.is_some() {
                                return Err(serde::de::Error::duplicate_field("deprecated"));
                            }
                            deprecated__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(schema::field::choice::Option {
                    value: value__,
                    label: label__.unwrap_or_default(),
                    description: description__.unwrap_or_default(),
                    deprecated: deprecated__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Choice.Option", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Computed {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.expr.is_empty() {
            len += 1;
        }
        if self.result.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Computed", len)?;
        if !self.expr.is_empty() {
            struct_ser.serialize_field("expr", &self.expr)?;
        }
        if let Some(v) = self.result.as_ref() {
            let v = schema::field::ResultType::try_from(*v)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", *v)))?;
            struct_ser.serialize_field("result", &v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Computed {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "expr",
            "result",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Expr,
            Result,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "expr" => Ok(GeneratedField::Expr),
                            "result" => Ok(GeneratedField::Result),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Computed;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Computed")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Computed, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut expr__ = None;
                let mut result__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Expr => {
                            if expr__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expr"));
                            }
                            expr__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Result => {
                            if result__.is_some() {
                                return Err(serde::de::Error::duplicate_field("result"));
                            }
                            result__ = map_.next_value::<::std::option::Option<schema::field::ResultType>>()?.map(|x| x as i32);
                        }
                    }
                }
                Ok(schema::field::Computed {
                    expr: expr__.unwrap_or_default(),
                    result: result__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Computed", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Double {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        if self.multiple_of.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Double", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.r#const.as_ref() {
            struct_ser.serialize_field("const", v)?;
        }
        if let Some(v) = self.gt.as_ref() {
            struct_ser.serialize_field("gt", v)?;
        }
        if let Some(v) = self.gte.as_ref() {
            struct_ser.serialize_field("gte", v)?;
        }
        if let Some(v) = self.lt.as_ref() {
            struct_ser.serialize_field("lt", v)?;
        }
        if let Some(v) = self.lte.as_ref() {
            struct_ser.serialize_field("lte", v)?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in)?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in)?;
        }
        if let Some(v) = self.multiple_of.as_ref() {
            struct_ser.serialize_field("multipleOf", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Double {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "not_in",
            "notIn",
            "multiple_of",
            "multipleOf",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Gt,
            Gte,
            Lt,
            Lte,
            In,
            NotIn,
            MultipleOf,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            "multipleOf" | "multiple_of" => Ok(GeneratedField::MultipleOf),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Double;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Double")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Double, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                let mut multiple_of__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::MultipleOf => {
                            if multiple_of__.is_some() {
                                return Err(serde::de::Error::duplicate_field("multipleOf"));
                            }
                            multiple_of__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                    }
                }
                Ok(schema::field::Double {
                    default: default__,
                    r#const: r#const__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                    multiple_of: multiple_of__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Double", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Duration {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Duration", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.gt.as_ref() {
            struct_ser.serialize_field("gt", v)?;
        }
        if let Some(v) = self.gte.as_ref() {
            struct_ser.serialize_field("gte", v)?;
        }
        if let Some(v) = self.lt.as_ref() {
            struct_ser.serialize_field("lt", v)?;
        }
        if let Some(v) = self.lte.as_ref() {
            struct_ser.serialize_field("lte", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Duration {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "gt",
            "gte",
            "lt",
            "lte",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Gt,
            Gte,
            Lt,
            Lte,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Duration;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Duration")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Duration, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = map_.next_value()?;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = map_.next_value()?;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = map_.next_value()?;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = map_.next_value()?;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::Duration {
                    default: default__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Duration", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Float {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        if self.multiple_of.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Float", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.r#const.as_ref() {
            struct_ser.serialize_field("const", v)?;
        }
        if let Some(v) = self.gt.as_ref() {
            struct_ser.serialize_field("gt", v)?;
        }
        if let Some(v) = self.gte.as_ref() {
            struct_ser.serialize_field("gte", v)?;
        }
        if let Some(v) = self.lt.as_ref() {
            struct_ser.serialize_field("lt", v)?;
        }
        if let Some(v) = self.lte.as_ref() {
            struct_ser.serialize_field("lte", v)?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in)?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in)?;
        }
        if let Some(v) = self.multiple_of.as_ref() {
            struct_ser.serialize_field("multipleOf", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Float {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "not_in",
            "notIn",
            "multiple_of",
            "multipleOf",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Gt,
            Gte,
            Lt,
            Lte,
            In,
            NotIn,
            MultipleOf,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            "multipleOf" | "multiple_of" => Ok(GeneratedField::MultipleOf),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Float;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Float")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Float, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                let mut multiple_of__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::MultipleOf => {
                            if multiple_of__.is_some() {
                                return Err(serde::de::Error::duplicate_field("multipleOf"));
                            }
                            multiple_of__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                    }
                }
                Ok(schema::field::Float {
                    default: default__,
                    r#const: r#const__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                    multiple_of: multiple_of__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Float", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Int32 {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        if self.multiple_of.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Int32", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.r#const.as_ref() {
            struct_ser.serialize_field("const", v)?;
        }
        if let Some(v) = self.gt.as_ref() {
            struct_ser.serialize_field("gt", v)?;
        }
        if let Some(v) = self.gte.as_ref() {
            struct_ser.serialize_field("gte", v)?;
        }
        if let Some(v) = self.lt.as_ref() {
            struct_ser.serialize_field("lt", v)?;
        }
        if let Some(v) = self.lte.as_ref() {
            struct_ser.serialize_field("lte", v)?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in)?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in)?;
        }
        if let Some(v) = self.multiple_of.as_ref() {
            struct_ser.serialize_field("multipleOf", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Int32 {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "not_in",
            "notIn",
            "multiple_of",
            "multipleOf",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Gt,
            Gte,
            Lt,
            Lte,
            In,
            NotIn,
            MultipleOf,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            "multipleOf" | "multiple_of" => Ok(GeneratedField::MultipleOf),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Int32;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Int32")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Int32, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                let mut multiple_of__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::MultipleOf => {
                            if multiple_of__.is_some() {
                                return Err(serde::de::Error::duplicate_field("multipleOf"));
                            }
                            multiple_of__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                    }
                }
                Ok(schema::field::Int32 {
                    default: default__,
                    r#const: r#const__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                    multiple_of: multiple_of__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Int32", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Int64 {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        if self.multiple_of.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Int64", len)?;
        if let Some(v) = self.default.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("default", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.r#const.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("const", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.gt.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("gt", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.gte.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("gte", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.lt.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lt", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.lte.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lte", ToString::to_string(&v).as_str())?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in.iter().map(ToString::to_string).collect::<Vec<_>>())?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in.iter().map(ToString::to_string).collect::<Vec<_>>())?;
        }
        if let Some(v) = self.multiple_of.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("multipleOf", ToString::to_string(&v).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Int64 {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "not_in",
            "notIn",
            "multiple_of",
            "multipleOf",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Gt,
            Gte,
            Lt,
            Lte,
            In,
            NotIn,
            MultipleOf,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            "multipleOf" | "multiple_of" => Ok(GeneratedField::MultipleOf),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Int64;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Int64")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Int64, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                let mut multiple_of__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::MultipleOf => {
                            if multiple_of__.is_some() {
                                return Err(serde::de::Error::duplicate_field("multipleOf"));
                            }
                            multiple_of__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                    }
                }
                Ok(schema::field::Int64 {
                    default: default__,
                    r#const: r#const__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                    multiple_of: multiple_of__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Int64", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Json {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Json", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Json {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Json;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Json")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Json, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::Json {
                    default: default__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Json", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::List {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.items.is_empty() {
            len += 1;
        }
        if self.min_items.is_some() {
            len += 1;
        }
        if self.max_items.is_some() {
            len += 1;
        }
        if self.unique {
            len += 1;
        }
        if self.count_expr.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.List", len)?;
        if !self.items.is_empty() {
            struct_ser.serialize_field("items", &self.items)?;
        }
        if let Some(v) = self.min_items.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("minItems", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.max_items.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("maxItems", ToString::to_string(&v).as_str())?;
        }
        if self.unique {
            struct_ser.serialize_field("unique", &self.unique)?;
        }
        if let Some(v) = self.count_expr.as_ref() {
            struct_ser.serialize_field("countExpr", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::List {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "items",
            "min_items",
            "minItems",
            "max_items",
            "maxItems",
            "unique",
            "count_expr",
            "countExpr",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Items,
            MinItems,
            MaxItems,
            Unique,
            CountExpr,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "items" => Ok(GeneratedField::Items),
                            "minItems" | "min_items" => Ok(GeneratedField::MinItems),
                            "maxItems" | "max_items" => Ok(GeneratedField::MaxItems),
                            "unique" => Ok(GeneratedField::Unique),
                            "countExpr" | "count_expr" => Ok(GeneratedField::CountExpr),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::List;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.List")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::List, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut items__ = None;
                let mut min_items__ = None;
                let mut max_items__ = None;
                let mut unique__ = None;
                let mut count_expr__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Items => {
                            if items__.is_some() {
                                return Err(serde::de::Error::duplicate_field("items"));
                            }
                            items__ = Some(map_.next_value()?);
                        }
                        GeneratedField::MinItems => {
                            if min_items__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minItems"));
                            }
                            min_items__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::MaxItems => {
                            if max_items__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxItems"));
                            }
                            max_items__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Unique => {
                            if unique__.is_some() {
                                return Err(serde::de::Error::duplicate_field("unique"));
                            }
                            unique__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CountExpr => {
                            if count_expr__.is_some() {
                                return Err(serde::de::Error::duplicate_field("countExpr"));
                            }
                            count_expr__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::List {
                    items: items__.unwrap_or_default(),
                    min_items: min_items__,
                    max_items: max_items__,
                    unique: unique__.unwrap_or_default(),
                    count_expr: count_expr__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.List", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Map {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.value_schema.is_some() {
            len += 1;
        }
        if self.min_entries.is_some() {
            len += 1;
        }
        if self.max_entries.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Map", len)?;
        if let Some(v) = self.value_schema.as_ref() {
            struct_ser.serialize_field("valueSchema", v)?;
        }
        if let Some(v) = self.min_entries.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("minEntries", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.max_entries.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("maxEntries", ToString::to_string(&v).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Map {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "value_schema",
            "valueSchema",
            "min_entries",
            "minEntries",
            "max_entries",
            "maxEntries",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ValueSchema,
            MinEntries,
            MaxEntries,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "valueSchema" | "value_schema" => Ok(GeneratedField::ValueSchema),
                            "minEntries" | "min_entries" => Ok(GeneratedField::MinEntries),
                            "maxEntries" | "max_entries" => Ok(GeneratedField::MaxEntries),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Map;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Map")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Map, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut value_schema__ = None;
                let mut min_entries__ = None;
                let mut max_entries__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ValueSchema => {
                            if value_schema__.is_some() {
                                return Err(serde::de::Error::duplicate_field("valueSchema"));
                            }
                            value_schema__ = map_.next_value()?;
                        }
                        GeneratedField::MinEntries => {
                            if min_entries__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minEntries"));
                            }
                            min_entries__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::MaxEntries => {
                            if max_entries__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxEntries"));
                            }
                            max_entries__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                    }
                }
                Ok(schema::field::Map {
                    value_schema: value_schema__,
                    min_entries: min_entries__,
                    max_entries: max_entries__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Map", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Object {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.schema.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Object", len)?;
        if let Some(v) = self.schema.as_ref() {
            struct_ser.serialize_field("schema", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Object {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "schema",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Schema,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "schema" => Ok(GeneratedField::Schema),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Object;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Object")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Object, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut schema__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Schema => {
                            if schema__.is_some() {
                                return Err(serde::de::Error::duplicate_field("schema"));
                            }
                            schema__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::Object {
                    schema: schema__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Object", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::OneOf {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.discriminator.is_empty() {
            len += 1;
        }
        if !self.variants.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.OneOf", len)?;
        if !self.discriminator.is_empty() {
            struct_ser.serialize_field("discriminator", &self.discriminator)?;
        }
        if !self.variants.is_empty() {
            struct_ser.serialize_field("variants", &self.variants)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::OneOf {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "discriminator",
            "variants",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Discriminator,
            Variants,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "discriminator" => Ok(GeneratedField::Discriminator),
                            "variants" => Ok(GeneratedField::Variants),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::OneOf;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.OneOf")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::OneOf, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut discriminator__ = None;
                let mut variants__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Discriminator => {
                            if discriminator__.is_some() {
                                return Err(serde::de::Error::duplicate_field("discriminator"));
                            }
                            discriminator__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Variants => {
                            if variants__.is_some() {
                                return Err(serde::de::Error::duplicate_field("variants"));
                            }
                            variants__ = Some(
                                map_.next_value::<std::collections::HashMap<_, _>>()?
                            );
                        }
                    }
                }
                Ok(schema::field::OneOf {
                    discriminator: discriminator__.unwrap_or_default(),
                    variants: variants__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.OneOf", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Ref {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.target.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Ref", len)?;
        if let Some(v) = self.target.as_ref() {
            match v {
                schema::field::r#ref::Target::Name(v) => {
                    struct_ser.serialize_field("name", v)?;
                }
                schema::field::r#ref::Target::Id(v) => {
                    struct_ser.serialize_field("id", v)?;
                }
            }
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Ref {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "name",
            "id",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Name,
            Id,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "name" => Ok(GeneratedField::Name),
                            "id" => Ok(GeneratedField::Id),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Ref;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Ref")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Ref, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut target__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Name => {
                            if target__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            target__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::r#ref::Target::Name);
                        }
                        GeneratedField::Id => {
                            if target__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            target__ = map_.next_value::<::std::option::Option<_>>()?.map(schema::field::r#ref::Target::Id)
;
                        }
                    }
                }
                Ok(schema::field::Ref {
                    target: target__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Ref", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::ResultType {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "RESULT_TYPE_UNSPECIFIED",
            Self::Double => "RESULT_TYPE_DOUBLE",
            Self::Int64 => "RESULT_TYPE_INT64",
            Self::Uint64 => "RESULT_TYPE_UINT64",
            Self::Bool => "RESULT_TYPE_BOOL",
            Self::String => "RESULT_TYPE_STRING",
            Self::Duration => "RESULT_TYPE_DURATION",
            Self::Timestamp => "RESULT_TYPE_TIMESTAMP",
            Self::Bytes => "RESULT_TYPE_BYTES",
            Self::Json => "RESULT_TYPE_JSON",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::ResultType {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "RESULT_TYPE_UNSPECIFIED",
            "RESULT_TYPE_DOUBLE",
            "RESULT_TYPE_INT64",
            "RESULT_TYPE_UINT64",
            "RESULT_TYPE_BOOL",
            "RESULT_TYPE_STRING",
            "RESULT_TYPE_DURATION",
            "RESULT_TYPE_TIMESTAMP",
            "RESULT_TYPE_BYTES",
            "RESULT_TYPE_JSON",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::ResultType;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "RESULT_TYPE_UNSPECIFIED" => Ok(schema::field::ResultType::Unspecified),
                    "RESULT_TYPE_DOUBLE" => Ok(schema::field::ResultType::Double),
                    "RESULT_TYPE_INT64" => Ok(schema::field::ResultType::Int64),
                    "RESULT_TYPE_UINT64" => Ok(schema::field::ResultType::Uint64),
                    "RESULT_TYPE_BOOL" => Ok(schema::field::ResultType::Bool),
                    "RESULT_TYPE_STRING" => Ok(schema::field::ResultType::String),
                    "RESULT_TYPE_DURATION" => Ok(schema::field::ResultType::Duration),
                    "RESULT_TYPE_TIMESTAMP" => Ok(schema::field::ResultType::Timestamp),
                    "RESULT_TYPE_BYTES" => Ok(schema::field::ResultType::Bytes),
                    "RESULT_TYPE_JSON" => Ok(schema::field::ResultType::Json),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Rule {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.expr.is_empty() {
            len += 1;
        }
        if !self.message.is_empty() {
            len += 1;
        }
        if self.id.is_some() {
            len += 1;
        }
        if self.severity.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Rule", len)?;
        if !self.expr.is_empty() {
            struct_ser.serialize_field("expr", &self.expr)?;
        }
        if !self.message.is_empty() {
            struct_ser.serialize_field("message", &self.message)?;
        }
        if let Some(v) = self.id.as_ref() {
            struct_ser.serialize_field("id", v)?;
        }
        if let Some(v) = self.severity.as_ref() {
            let v = schema::field::Severity::try_from(*v)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", *v)))?;
            struct_ser.serialize_field("severity", &v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Rule {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "expr",
            "message",
            "id",
            "severity",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Expr,
            Message,
            Id,
            Severity,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "expr" => Ok(GeneratedField::Expr),
                            "message" => Ok(GeneratedField::Message),
                            "id" => Ok(GeneratedField::Id),
                            "severity" => Ok(GeneratedField::Severity),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Rule;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Rule")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Rule, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut expr__ = None;
                let mut message__ = None;
                let mut id__ = None;
                let mut severity__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Expr => {
                            if expr__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expr"));
                            }
                            expr__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Message => {
                            if message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("message"));
                            }
                            message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = map_.next_value()?;
                        }
                        GeneratedField::Severity => {
                            if severity__.is_some() {
                                return Err(serde::de::Error::duplicate_field("severity"));
                            }
                            severity__ = map_.next_value::<::std::option::Option<schema::field::Severity>>()?.map(|x| x as i32);
                        }
                    }
                }
                Ok(schema::field::Rule {
                    expr: expr__.unwrap_or_default(),
                    message: message__.unwrap_or_default(),
                    id: id__,
                    severity: severity__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Rule", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Severity {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "SEVERITY_UNSPECIFIED",
            Self::Error => "SEVERITY_ERROR",
            Self::Warning => "SEVERITY_WARNING",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Severity {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "SEVERITY_UNSPECIFIED",
            "SEVERITY_ERROR",
            "SEVERITY_WARNING",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Severity;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "SEVERITY_UNSPECIFIED" => Ok(schema::field::Severity::Unspecified),
                    "SEVERITY_ERROR" => Ok(schema::field::Severity::Error),
                    "SEVERITY_WARNING" => Ok(schema::field::Severity::Warning),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::String {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.len.is_some() {
            len += 1;
        }
        if self.min_len.is_some() {
            len += 1;
        }
        if self.max_len.is_some() {
            len += 1;
        }
        if self.pattern.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        if self.format.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.String", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.r#const.as_ref() {
            struct_ser.serialize_field("const", v)?;
        }
        if let Some(v) = self.len.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("len", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.min_len.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("minLen", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.max_len.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("maxLen", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.pattern.as_ref() {
            struct_ser.serialize_field("pattern", v)?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in)?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in)?;
        }
        if let Some(v) = self.format.as_ref() {
            struct_ser.serialize_field("format", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::String {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "len",
            "min_len",
            "minLen",
            "max_len",
            "maxLen",
            "pattern",
            "in",
            "not_in",
            "notIn",
            "format",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Len,
            MinLen,
            MaxLen,
            Pattern,
            In,
            NotIn,
            Format,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "len" => Ok(GeneratedField::Len),
                            "minLen" | "min_len" => Ok(GeneratedField::MinLen),
                            "maxLen" | "max_len" => Ok(GeneratedField::MaxLen),
                            "pattern" => Ok(GeneratedField::Pattern),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            "format" => Ok(GeneratedField::Format),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::String;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.String")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::String, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut len__ = None;
                let mut min_len__ = None;
                let mut max_len__ = None;
                let mut pattern__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                let mut format__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = map_.next_value()?;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = map_.next_value()?;
                        }
                        GeneratedField::Len => {
                            if len__.is_some() {
                                return Err(serde::de::Error::duplicate_field("len"));
                            }
                            len__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::MinLen => {
                            if min_len__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minLen"));
                            }
                            min_len__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::MaxLen => {
                            if max_len__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxLen"));
                            }
                            max_len__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Pattern => {
                            if pattern__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pattern"));
                            }
                            pattern__ = map_.next_value()?;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Format => {
                            if format__.is_some() {
                                return Err(serde::de::Error::duplicate_field("format"));
                            }
                            format__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::String {
                    default: default__,
                    r#const: r#const__,
                    len: len__,
                    min_len: min_len__,
                    max_len: max_len__,
                    pattern: pattern__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                    format: format__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.String", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::Timestamp {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.Timestamp", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.gt.as_ref() {
            struct_ser.serialize_field("gt", v)?;
        }
        if let Some(v) = self.gte.as_ref() {
            struct_ser.serialize_field("gte", v)?;
        }
        if let Some(v) = self.lt.as_ref() {
            struct_ser.serialize_field("lt", v)?;
        }
        if let Some(v) = self.lte.as_ref() {
            struct_ser.serialize_field("lte", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::Timestamp {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "gt",
            "gte",
            "lt",
            "lte",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Gt,
            Gte,
            Lt,
            Lte,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::Timestamp;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.Timestamp")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::Timestamp, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = map_.next_value()?;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = map_.next_value()?;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = map_.next_value()?;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = map_.next_value()?;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = map_.next_value()?;
                        }
                    }
                }
                Ok(schema::field::Timestamp {
                    default: default__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.Timestamp", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::UInt32 {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        if self.multiple_of.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.UInt32", len)?;
        if let Some(v) = self.default.as_ref() {
            struct_ser.serialize_field("default", v)?;
        }
        if let Some(v) = self.r#const.as_ref() {
            struct_ser.serialize_field("const", v)?;
        }
        if let Some(v) = self.gt.as_ref() {
            struct_ser.serialize_field("gt", v)?;
        }
        if let Some(v) = self.gte.as_ref() {
            struct_ser.serialize_field("gte", v)?;
        }
        if let Some(v) = self.lt.as_ref() {
            struct_ser.serialize_field("lt", v)?;
        }
        if let Some(v) = self.lte.as_ref() {
            struct_ser.serialize_field("lte", v)?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in)?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in)?;
        }
        if let Some(v) = self.multiple_of.as_ref() {
            struct_ser.serialize_field("multipleOf", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::UInt32 {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "not_in",
            "notIn",
            "multiple_of",
            "multipleOf",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Gt,
            Gte,
            Lt,
            Lte,
            In,
            NotIn,
            MultipleOf,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            "multipleOf" | "multiple_of" => Ok(GeneratedField::MultipleOf),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::UInt32;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.UInt32")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::UInt32, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                let mut multiple_of__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::MultipleOf => {
                            if multiple_of__.is_some() {
                                return Err(serde::de::Error::duplicate_field("multipleOf"));
                            }
                            multiple_of__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                    }
                }
                Ok(schema::field::UInt32 {
                    default: default__,
                    r#const: r#const__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                    multiple_of: multiple_of__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.UInt32", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for schema::field::UInt64 {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.default.is_some() {
            len += 1;
        }
        if self.r#const.is_some() {
            len += 1;
        }
        if self.gt.is_some() {
            len += 1;
        }
        if self.gte.is_some() {
            len += 1;
        }
        if self.lt.is_some() {
            len += 1;
        }
        if self.lte.is_some() {
            len += 1;
        }
        if !self.r#in.is_empty() {
            len += 1;
        }
        if !self.not_in.is_empty() {
            len += 1;
        }
        if self.multiple_of.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Schema.Field.UInt64", len)?;
        if let Some(v) = self.default.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("default", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.r#const.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("const", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.gt.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("gt", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.gte.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("gte", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.lt.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lt", ToString::to_string(&v).as_str())?;
        }
        if let Some(v) = self.lte.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lte", ToString::to_string(&v).as_str())?;
        }
        if !self.r#in.is_empty() {
            struct_ser.serialize_field("in", &self.r#in.iter().map(ToString::to_string).collect::<Vec<_>>())?;
        }
        if !self.not_in.is_empty() {
            struct_ser.serialize_field("notIn", &self.not_in.iter().map(ToString::to_string).collect::<Vec<_>>())?;
        }
        if let Some(v) = self.multiple_of.as_ref() {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("multipleOf", ToString::to_string(&v).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for schema::field::UInt64 {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "default",
            "const",
            "gt",
            "gte",
            "lt",
            "lte",
            "in",
            "not_in",
            "notIn",
            "multiple_of",
            "multipleOf",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Default,
            Const,
            Gt,
            Gte,
            Lt,
            Lte,
            In,
            NotIn,
            MultipleOf,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "default" => Ok(GeneratedField::Default),
                            "const" => Ok(GeneratedField::Const),
                            "gt" => Ok(GeneratedField::Gt),
                            "gte" => Ok(GeneratedField::Gte),
                            "lt" => Ok(GeneratedField::Lt),
                            "lte" => Ok(GeneratedField::Lte),
                            "in" => Ok(GeneratedField::In),
                            "notIn" | "not_in" => Ok(GeneratedField::NotIn),
                            "multipleOf" | "multiple_of" => Ok(GeneratedField::MultipleOf),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = schema::field::UInt64;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Schema.Field.UInt64")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<schema::field::UInt64, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut default__ = None;
                let mut r#const__ = None;
                let mut gt__ = None;
                let mut gte__ = None;
                let mut lt__ = None;
                let mut lte__ = None;
                let mut r#in__ = None;
                let mut not_in__ = None;
                let mut multiple_of__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Default => {
                            if default__.is_some() {
                                return Err(serde::de::Error::duplicate_field("default"));
                            }
                            default__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Const => {
                            if r#const__.is_some() {
                                return Err(serde::de::Error::duplicate_field("const"));
                            }
                            r#const__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gt => {
                            if gt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gt"));
                            }
                            gt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Gte => {
                            if gte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("gte"));
                            }
                            gte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lt => {
                            if lt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lt"));
                            }
                            lt__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::Lte => {
                            if lte__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lte"));
                            }
                            lte__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                        GeneratedField::In => {
                            if r#in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("in"));
                            }
                            r#in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NotIn => {
                            if not_in__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notIn"));
                            }
                            not_in__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::MultipleOf => {
                            if multiple_of__.is_some() {
                                return Err(serde::de::Error::duplicate_field("multipleOf"));
                            }
                            multiple_of__ = 
                                map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| x.0)
                            ;
                        }
                    }
                }
                Ok(schema::field::UInt64 {
                    default: default__,
                    r#const: r#const__,
                    gt: gt__,
                    gte: gte__,
                    lt: lt__,
                    lte: lte__,
                    r#in: r#in__.unwrap_or_default(),
                    not_in: not_in__.unwrap_or_default(),
                    multiple_of: multiple_of__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Schema.Field.UInt64", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SchemaIdentity {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.namespace.is_empty() {
            len += 1;
        }
        if !self.name.is_empty() {
            len += 1;
        }
        if !self.version.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.SchemaIdentity", len)?;
        if !self.namespace.is_empty() {
            struct_ser.serialize_field("namespace", &self.namespace)?;
        }
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if !self.version.is_empty() {
            struct_ser.serialize_field("version", &self.version)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SchemaIdentity {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "namespace",
            "name",
            "version",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Namespace,
            Name,
            Version,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "namespace" => Ok(GeneratedField::Namespace),
                            "name" => Ok(GeneratedField::Name),
                            "version" => Ok(GeneratedField::Version),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SchemaIdentity;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.SchemaIdentity")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SchemaIdentity, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut namespace__ = None;
                let mut name__ = None;
                let mut version__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Namespace => {
                            if namespace__.is_some() {
                                return Err(serde::de::Error::duplicate_field("namespace"));
                            }
                            namespace__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Version => {
                            if version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("version"));
                            }
                            version__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(SchemaIdentity {
                    namespace: namespace__.unwrap_or_default(),
                    name: name__.unwrap_or_default(),
                    version: version__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.SchemaIdentity", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SchemaRef {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.source.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.SchemaRef", len)?;
        if let Some(v) = self.source.as_ref() {
            match v {
                schema_ref::Source::Id(v) => {
                    struct_ser.serialize_field("id", v)?;
                }
                schema_ref::Source::Schema(v) => {
                    struct_ser.serialize_field("schema", v)?;
                }
            }
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SchemaRef {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "schema",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            Schema,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "id" => Ok(GeneratedField::Id),
                            "schema" => Ok(GeneratedField::Schema),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SchemaRef;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.SchemaRef")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SchemaRef, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut source__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if source__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            source__ = map_.next_value::<::std::option::Option<_>>()?.map(schema_ref::Source::Id)
;
                        }
                        GeneratedField::Schema => {
                            if source__.is_some() {
                                return Err(serde::de::Error::duplicate_field("schema"));
                            }
                            source__ = map_.next_value::<::std::option::Option<_>>()?.map(schema_ref::Source::Schema)
;
                        }
                    }
                }
                Ok(SchemaRef {
                    source: source__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.SchemaRef", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for StructValue {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.fields.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.StructValue", len)?;
        if !self.fields.is_empty() {
            struct_ser.serialize_field("fields", &self.fields)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StructValue {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "fields",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Fields,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "fields" => Ok(GeneratedField::Fields),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StructValue;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.StructValue")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StructValue, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut fields__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Fields => {
                            if fields__.is_some() {
                                return Err(serde::de::Error::duplicate_field("fields"));
                            }
                            fields__ = Some(
                                map_.next_value::<std::collections::HashMap<_, _>>()?
                            );
                        }
                    }
                }
                Ok(StructValue {
                    fields: fields__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.StructValue", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ValidationError {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.path.is_empty() {
            len += 1;
        }
        if self.code != 0 {
            len += 1;
        }
        if self.expected.is_some() {
            len += 1;
        }
        if self.actual.is_some() {
            len += 1;
        }
        if !self.constraint.is_empty() {
            len += 1;
        }
        if !self.expr.is_empty() {
            len += 1;
        }
        if self.rule_id.is_some() {
            len += 1;
        }
        if self.severity != 0 {
            len += 1;
        }
        if !self.message.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.ValidationError", len)?;
        if !self.path.is_empty() {
            struct_ser.serialize_field("path", &self.path)?;
        }
        if self.code != 0 {
            let v = ErrorCode::try_from(self.code)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.code)))?;
            struct_ser.serialize_field("code", &v)?;
        }
        if let Some(v) = self.expected.as_ref() {
            struct_ser.serialize_field("expected", v)?;
        }
        if let Some(v) = self.actual.as_ref() {
            struct_ser.serialize_field("actual", v)?;
        }
        if !self.constraint.is_empty() {
            struct_ser.serialize_field("constraint", &self.constraint)?;
        }
        if !self.expr.is_empty() {
            struct_ser.serialize_field("expr", &self.expr)?;
        }
        if let Some(v) = self.rule_id.as_ref() {
            struct_ser.serialize_field("ruleId", v)?;
        }
        if self.severity != 0 {
            let v = schema::field::Severity::try_from(self.severity)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.severity)))?;
            struct_ser.serialize_field("severity", &v)?;
        }
        if !self.message.is_empty() {
            struct_ser.serialize_field("message", &self.message)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ValidationError {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "path",
            "code",
            "expected",
            "actual",
            "constraint",
            "expr",
            "rule_id",
            "ruleId",
            "severity",
            "message",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Path,
            Code,
            Expected,
            Actual,
            Constraint,
            Expr,
            RuleId,
            Severity,
            Message,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "path" => Ok(GeneratedField::Path),
                            "code" => Ok(GeneratedField::Code),
                            "expected" => Ok(GeneratedField::Expected),
                            "actual" => Ok(GeneratedField::Actual),
                            "constraint" => Ok(GeneratedField::Constraint),
                            "expr" => Ok(GeneratedField::Expr),
                            "ruleId" | "rule_id" => Ok(GeneratedField::RuleId),
                            "severity" => Ok(GeneratedField::Severity),
                            "message" => Ok(GeneratedField::Message),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ValidationError;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.ValidationError")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ValidationError, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut path__ = None;
                let mut code__ = None;
                let mut expected__ = None;
                let mut actual__ = None;
                let mut constraint__ = None;
                let mut expr__ = None;
                let mut rule_id__ = None;
                let mut severity__ = None;
                let mut message__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Path => {
                            if path__.is_some() {
                                return Err(serde::de::Error::duplicate_field("path"));
                            }
                            path__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Code => {
                            if code__.is_some() {
                                return Err(serde::de::Error::duplicate_field("code"));
                            }
                            code__ = Some(map_.next_value::<ErrorCode>()? as i32);
                        }
                        GeneratedField::Expected => {
                            if expected__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expected"));
                            }
                            expected__ = map_.next_value()?;
                        }
                        GeneratedField::Actual => {
                            if actual__.is_some() {
                                return Err(serde::de::Error::duplicate_field("actual"));
                            }
                            actual__ = map_.next_value()?;
                        }
                        GeneratedField::Constraint => {
                            if constraint__.is_some() {
                                return Err(serde::de::Error::duplicate_field("constraint"));
                            }
                            constraint__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Expr => {
                            if expr__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expr"));
                            }
                            expr__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleId => {
                            if rule_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleId"));
                            }
                            rule_id__ = map_.next_value()?;
                        }
                        GeneratedField::Severity => {
                            if severity__.is_some() {
                                return Err(serde::de::Error::duplicate_field("severity"));
                            }
                            severity__ = Some(map_.next_value::<schema::field::Severity>()? as i32);
                        }
                        GeneratedField::Message => {
                            if message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("message"));
                            }
                            message__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ValidationError {
                    path: path__.unwrap_or_default(),
                    code: code__.unwrap_or_default(),
                    expected: expected__,
                    actual: actual__,
                    constraint: constraint__.unwrap_or_default(),
                    expr: expr__.unwrap_or_default(),
                    rule_id: rule_id__,
                    severity: severity__.unwrap_or_default(),
                    message: message__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.ValidationError", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ValidationResult {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.errors.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.ValidationResult", len)?;
        if !self.errors.is_empty() {
            struct_ser.serialize_field("errors", &self.errors)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ValidationResult {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "errors",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Errors,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "errors" => Ok(GeneratedField::Errors),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ValidationResult;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.ValidationResult")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ValidationResult, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut errors__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Errors => {
                            if errors__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errors"));
                            }
                            errors__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ValidationResult {
                    errors: errors__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("schemapb.ValidationResult", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Value {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.kind.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("schemapb.Value", len)?;
        if let Some(v) = self.kind.as_ref() {
            match v {
                value::Kind::NullValue(v) => {
                    let v = NullValue::try_from(*v)
                        .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", *v)))?;
                    struct_ser.serialize_field("nullValue", &v)?;
                }
                value::Kind::BoolValue(v) => {
                    struct_ser.serialize_field("boolValue", v)?;
                }
                value::Kind::Int32Value(v) => {
                    struct_ser.serialize_field("int32Value", v)?;
                }
                value::Kind::Int64Value(v) => {
                    #[allow(clippy::needless_borrow)]
                    #[allow(clippy::needless_borrows_for_generic_args)]
                    struct_ser.serialize_field("int64Value", ToString::to_string(&v).as_str())?;
                }
                value::Kind::Uint32Value(v) => {
                    struct_ser.serialize_field("uint32Value", v)?;
                }
                value::Kind::Uint64Value(v) => {
                    #[allow(clippy::needless_borrow)]
                    #[allow(clippy::needless_borrows_for_generic_args)]
                    struct_ser.serialize_field("uint64Value", ToString::to_string(&v).as_str())?;
                }
                value::Kind::FloatValue(v) => {
                    struct_ser.serialize_field("floatValue", v)?;
                }
                value::Kind::DoubleValue(v) => {
                    struct_ser.serialize_field("doubleValue", v)?;
                }
                value::Kind::StringValue(v) => {
                    struct_ser.serialize_field("stringValue", v)?;
                }
                value::Kind::DurationValue(v) => {
                    struct_ser.serialize_field("durationValue", v)?;
                }
                value::Kind::TimestampValue(v) => {
                    struct_ser.serialize_field("timestampValue", v)?;
                }
                value::Kind::ListValue(v) => {
                    struct_ser.serialize_field("listValue", v)?;
                }
                value::Kind::StructValue(v) => {
                    struct_ser.serialize_field("structValue", v)?;
                }
                value::Kind::BytesValue(v) => {
                    #[allow(clippy::needless_borrow)]
                    #[allow(clippy::needless_borrows_for_generic_args)]
                    struct_ser.serialize_field("bytesValue", pbjson::private::base64::encode(&v).as_str())?;
                }
            }
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Value {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "null_value",
            "nullValue",
            "bool_value",
            "boolValue",
            "int32_value",
            "int32Value",
            "int64_value",
            "int64Value",
            "uint32_value",
            "uint32Value",
            "uint64_value",
            "uint64Value",
            "float_value",
            "floatValue",
            "double_value",
            "doubleValue",
            "string_value",
            "stringValue",
            "duration_value",
            "durationValue",
            "timestamp_value",
            "timestampValue",
            "list_value",
            "listValue",
            "struct_value",
            "structValue",
            "bytes_value",
            "bytesValue",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            NullValue,
            BoolValue,
            Int32Value,
            Int64Value,
            Uint32Value,
            Uint64Value,
            FloatValue,
            DoubleValue,
            StringValue,
            DurationValue,
            TimestampValue,
            ListValue,
            StructValue,
            BytesValue,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "nullValue" | "null_value" => Ok(GeneratedField::NullValue),
                            "boolValue" | "bool_value" => Ok(GeneratedField::BoolValue),
                            "int32Value" | "int32_value" => Ok(GeneratedField::Int32Value),
                            "int64Value" | "int64_value" => Ok(GeneratedField::Int64Value),
                            "uint32Value" | "uint32_value" => Ok(GeneratedField::Uint32Value),
                            "uint64Value" | "uint64_value" => Ok(GeneratedField::Uint64Value),
                            "floatValue" | "float_value" => Ok(GeneratedField::FloatValue),
                            "doubleValue" | "double_value" => Ok(GeneratedField::DoubleValue),
                            "stringValue" | "string_value" => Ok(GeneratedField::StringValue),
                            "durationValue" | "duration_value" => Ok(GeneratedField::DurationValue),
                            "timestampValue" | "timestamp_value" => Ok(GeneratedField::TimestampValue),
                            "listValue" | "list_value" => Ok(GeneratedField::ListValue),
                            "structValue" | "struct_value" => Ok(GeneratedField::StructValue),
                            "bytesValue" | "bytes_value" => Ok(GeneratedField::BytesValue),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Value;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct schemapb.Value")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Value, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut kind__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::NullValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nullValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<NullValue>>()?.map(|x| value::Kind::NullValue(x as i32));
                        }
                        GeneratedField::BoolValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("boolValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(value::Kind::BoolValue);
                        }
                        GeneratedField::Int32Value => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("int32Value"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| value::Kind::Int32Value(x.0));
                        }
                        GeneratedField::Int64Value => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("int64Value"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| value::Kind::Int64Value(x.0));
                        }
                        GeneratedField::Uint32Value => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("uint32Value"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| value::Kind::Uint32Value(x.0));
                        }
                        GeneratedField::Uint64Value => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("uint64Value"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| value::Kind::Uint64Value(x.0));
                        }
                        GeneratedField::FloatValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("floatValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| value::Kind::FloatValue(x.0));
                        }
                        GeneratedField::DoubleValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("doubleValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| value::Kind::DoubleValue(x.0));
                        }
                        GeneratedField::StringValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stringValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(value::Kind::StringValue);
                        }
                        GeneratedField::DurationValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("durationValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(value::Kind::DurationValue)
;
                        }
                        GeneratedField::TimestampValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timestampValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(value::Kind::TimestampValue)
;
                        }
                        GeneratedField::ListValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("listValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(value::Kind::ListValue)
;
                        }
                        GeneratedField::StructValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("structValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<_>>()?.map(value::Kind::StructValue)
;
                        }
                        GeneratedField::BytesValue => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bytesValue"));
                            }
                            kind__ = map_.next_value::<::std::option::Option<::pbjson::private::BytesDeserialize<_>>>()?.map(|x| value::Kind::BytesValue(x.0));
                        }
                    }
                }
                Ok(Value {
                    kind: kind__,
                })
            }
        }
        deserializer.deserialize_struct("schemapb.Value", FIELDS, GeneratedVisitor)
    }
}
