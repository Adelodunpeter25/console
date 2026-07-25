import Foundation

/// A type-erased JSON value used for the dynamic portions of the API
/// (e.g. `arguments`, `content`, arbitrary request/response payloads).
public enum JSONValue: Sendable, Hashable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])
}

extension JSONValue: Codable {
    public init(from decoder: any Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let bool = try? container.decode(Bool.self) {
            self = .bool(bool)
        } else if let number = try? container.decode(Double.self) {
            self = .number(number)
        } else if let string = try? container.decode(String.self) {
            self = .string(string)
        } else if let array = try? container.decode([JSONValue].self) {
            self = .array(array)
        } else if let object = try? container.decode([String: JSONValue].self) {
            self = .object(object)
        } else {
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "Unsupported JSON value"
            )
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .null:      try container.encodeNil()
        case .bool(let v):  try container.encode(v)
        case .number(let v): try container.encode(v)
        case .string(let v): try container.encode(v)
        case .array(let v):  try container.encode(v)
        case .object(let v): try container.encode(v)
        }
    }
}

public extension JSONValue {
    subscript(key: String) -> JSONValue? {
        if case .object(let dict) = self { return dict[key] }
        return nil
    }

    var stringValue: String? { if case .string(let v) = self { return v }; return nil }
    var doubleValue: Double? { if case .number(let v) = self { return v }; return nil }
    var intValue: Int?       { if case .number(let v) = self { return Int(v) }; return nil }
    var boolValue: Bool?     { if case .bool(let v) = self { return v }; return nil }
    var arrayValue: [JSONValue]? { if case .array(let v) = self { return v }; return nil }
    var objectValue: [String: JSONValue]? { if case .object(let v) = self { return v }; return nil }

    /// Pretty-printed, human-readable JSON representation (2-space indent).
    /// Falls back to a compact string for empty values.
    var prettyPrinted: String {
        do {
            let data = try JSONEncoder().encode(self)
            let object = try JSONSerialization.jsonObject(with: data, options: [.allowFragments])
            let pretty = try JSONSerialization.data(
                withJSONObject: object,
                options: [.prettyPrinted, .fragmentsAllowed]
            )
            return String(data: pretty, encoding: .utf8) ?? "\(self)"
        } catch {
            return "\(self)"
        }
    }
}

extension JSONValue: ExpressibleByStringLiteral     { public init(stringLiteral v: String) { self = .string(v) } }
extension JSONValue: ExpressibleByBooleanLiteral    { public init(booleanLiteral v: Bool) { self = .bool(v) } }
extension JSONValue: ExpressibleByIntegerLiteral     { public init(integerLiteral v: Int) { self = .number(Double(v)) } }
extension JSONValue: ExpressibleByFloatLiteral       { public init(floatLiteral v: Double) { self = .number(v) } }
extension JSONValue: ExpressibleByNilLiteral         { public init(nilLiteral: ()) { self = .null } }
extension JSONValue: ExpressibleByArrayLiteral       { public init(arrayLiteral e: JSONValue...) { self = .array(e) } }
extension JSONValue: ExpressibleByDictionaryLiteral  { public init(dictionaryLiteral e: (String, JSONValue)...) { self = .object(Dictionary(uniqueKeysWithValues: e)) } }
