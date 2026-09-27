//! Escaped canonical paths for diagnostics and resolve reports.
use crate::gen::schemapb::{path_segment::Segment, PathSegment};

pub fn segments(mut path: &str) -> Vec<PathSegment> {
    let mut out = Vec::new();
    while !path.is_empty() {
        if let Some(rest) = path.strip_prefix('.') {
            path = rest;
            continue;
        }
        if let Some(rest) = path.strip_prefix('[') {
            path = rest;
            if path.starts_with('"') {
                let mut end = 1;
                let bytes = path.as_bytes();
                while end < bytes.len() {
                    if bytes[end] == b'\\' {
                        end += 2;
                        continue;
                    }
                    if bytes[end] == b'"' {
                        break;
                    }
                    end += 1;
                }
                if end + 1 >= bytes.len() || bytes[end + 1] != b']' {
                    return Vec::new();
                }
                let Ok(key) = serde_json::from_str::<String>(&path[..=end]) else {
                    return Vec::new();
                };
                out.push(PathSegment {
                    segment: Some(Segment::Key(key)),
                });
                path = &path[end + 2..];
            } else {
                let Some(end) = path.find(']') else {
                    return Vec::new();
                };
                let Ok(index) = path[..end].parse() else {
                    return Vec::new();
                };
                out.push(PathSegment {
                    segment: Some(Segment::Index(index)),
                });
                path = &path[end + 1..];
            }
        } else {
            let end = path.find(['.', '[']).unwrap_or(path.len());
            out.push(PathSegment {
                segment: Some(Segment::Key(path[..end].into())),
            });
            path = &path[end..];
        }
    }
    out
}
