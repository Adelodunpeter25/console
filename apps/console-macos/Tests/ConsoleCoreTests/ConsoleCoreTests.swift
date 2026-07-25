import Testing
@testable import ConsoleCore

@Test func decodeApiResponse() throws {
    let json = #"""
    {"success":true,"data":{"loggedIn":true,"email":"a@b.com","projectId":"p1"},"error":null}
    """#.data(using: .utf8)!

    let resp = try JSONDecoder().decode(ApiResponse<ProviderAuthStatus>.self, from: json)
    #expect(resp.success == true)
    #expect(resp.data?.loggedIn == true)
    #expect(resp.data?.email == "a@b.com")
}

@Test func decodeAgentMessageUser() throws {
    let json = #"{"role":"user","content":"hello"}"#.data(using: .utf8)!
    let msg = try JSONDecoder().decode(AgentMessage.self, from: json)
    if case .user(let content) = msg {
        #expect(content == "hello")
    } else {
        Issue.record("expected user variant")
    }
}

@Test func decodeAgentMessageAssistantWithToolCall() throws {
    let json = #"""
    {"role":"assistant","id":"m1","content":[{"type":"toolCall","call":{"id":"tc1","name":"read","arguments":{"path":"/x"}}}],"stopReason":"tool_use"}
    """#.data(using: .utf8)!
    let msg = try JSONDecoder().decode(AgentMessage.self, from: json)
    if case .assistant(let id, let content, let stopReason) = msg {
        #expect(id == "m1")
        #expect(stopReason == "tool_use")
        #expect(content.count == 1)
        if case .toolCall(let call) = content[0] {
            #expect(call.name == "read")
        } else {
            Issue.record("expected toolCall content")
        }
    } else {
        Issue.record("expected assistant variant")
    }
}

@Test func decodeSessionEvent() throws {
    let json = #"{"type":"turnStart","prompt":"do it"}"#.data(using: .utf8)!
    let event = try JSONDecoder().decode(AgentSessionEvent.self, from: json)
    if case .turnStart(let prompt) = event {
        #expect(prompt == "do it")
    } else {
        Issue.record("expected turnStart")
    }
}
