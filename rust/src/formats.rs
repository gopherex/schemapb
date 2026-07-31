//! The spec's string-format registry: mandatory core formats, extensible per
//! engine; an unknown format fails validation loudly.

use std::collections::HashMap;
use std::sync::LazyLock;

pub type FormatFunc = fn(&str) -> bool;

/// Boxed checkers so extensions can capture state.
pub type FormatRegistry = HashMap<String, Box<dyn Fn(&str) -> bool + Send + Sync>>;

static UUID_RE: LazyLock<regex::Regex> = LazyLock::new(|| {
    regex::Regex::new(r"(?i)^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$").unwrap()
});
static HOSTNAME_RE: LazyLock<regex::Regex> = LazyLock::new(|| {
    regex::Regex::new(
        r"(?i)^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$",
    )
    .unwrap()
});
static EMAIL_RE: LazyLock<regex::Regex> =
    LazyLock::new(|| regex::Regex::new(r"^[^@\s]+@[^@\s]+$").unwrap());
static DATE_RE: LazyLock<regex::Regex> =
    LazyLock::new(|| regex::Regex::new(r"^\d{4}-\d{2}-\d{2}$").unwrap());
static TIME_RE: LazyLock<regex::Regex> =
    LazyLock::new(|| regex::Regex::new(r"^\d{2}:\d{2}:\d{2}$").unwrap());
static RFC3339_RE: LazyLock<regex::Regex> = LazyLock::new(|| {
    regex::Regex::new(r"^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$")
        .unwrap()
});
static URL_RE: LazyLock<regex::Regex> =
    LazyLock::new(|| regex::Regex::new(r"^[a-zA-Z][a-zA-Z0-9+.-]*://\S+$|^/\S*$").unwrap());

fn is_ipv4(s: &str) -> bool {
    let octets: Vec<&str> = s.split('.').collect();
    octets.len() == 4
        && octets.iter().all(|o| {
            !o.is_empty()
                && o.len() <= 3
                && !(o.len() > 1 && o.starts_with('0'))
                && o.chars().all(|c| c.is_ascii_digit())
                && o.parse::<u16>().is_ok_and(|n| n <= 255)
        })
}

fn is_ipv6(s: &str) -> bool {
    if !s.contains(':') || s.contains(' ') || s.matches("::").count() > 1 {
        return false;
    }
    let groups: Vec<&str> = s
        .split("::")
        .flat_map(|part| part.split(':'))
        .filter(|g| !g.is_empty())
        .collect();
    if groups.len() > 8 || (!s.contains("::") && groups.len() != 8) {
        return false;
    }
    groups
        .iter()
        .all(|g| (g.len() <= 4 && g.chars().all(|c| c.is_ascii_hexdigit())) || is_ipv4(g))
}

fn is_date(s: &str) -> bool {
    DATE_RE.is_match(s) && chrono::NaiveDate::parse_from_str(s, "%Y-%m-%d").is_ok()
}

fn is_time(s: &str) -> bool {
    TIME_RE.is_match(s) && chrono::NaiveTime::parse_from_str(s, "%H:%M:%S").is_ok()
}

fn is_datetime(s: &str) -> bool {
    RFC3339_RE.is_match(s) && chrono::DateTime::parse_from_rfc3339(s).is_ok()
}

/// A fresh registry with the spec's mandatory core formats.
#[must_use]
pub fn core_formats() -> FormatRegistry {
    let mut m: FormatRegistry = HashMap::new();
    m.insert("email".into(), Box::new(|s| EMAIL_RE.is_match(s)));
    m.insert("url".into(), Box::new(|s| URL_RE.is_match(s)));
    m.insert("uuid".into(), Box::new(|s| UUID_RE.is_match(s)));
    m.insert("ipv4".into(), Box::new(is_ipv4));
    m.insert("ipv6".into(), Box::new(|s| is_ipv6(s) && !is_ipv4(s)));
    m.insert("ip".into(), Box::new(|s| is_ipv4(s) || is_ipv6(s)));
    m.insert(
        "hostname".into(),
        Box::new(|s| s.len() <= 253 && HOSTNAME_RE.is_match(s)),
    );
    m.insert("date".into(), Box::new(is_date));
    m.insert("time".into(), Box::new(is_time));
    m.insert("datetime".into(), Box::new(is_datetime));
    m
}
