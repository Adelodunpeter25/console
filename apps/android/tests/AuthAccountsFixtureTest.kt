package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AuthStatusResponse
import console.v1.OAuthCallbackResponse
import console.v1.ProviderAccount
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the multi-account auth payloads (docs/plan/
 * provider-accounts.md Phase 3). The fixtures live in proto/testdata/auth
 * and are shared by Go, Rust, and Kotlin. Beyond decoding, these pin the
 * product decisions on the wire: active/env arrive as explicit values,
 * legacy payloads without accounts still decode, and the synthetic env
 * account is reported active and locked.
 */
class AuthAccountsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val statusAdapter = moshi.adapter(AuthStatusResponse::class.java)
    private val callbackAdapter = moshi.adapter(OAuthCallbackResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/auth/$name").readText().trim()

    @Test
    fun decodesStatusWithAccounts() {
        val data = statusAdapter.fromJson(fixture("status.json"))!!

        // antigravity: one inactive-account-only provider row.
        val antigravity = data.antigravity!!
        assertTrue(antigravity.logged_in)
        assertEquals("adelodunjoseph892@gmail.com", antigravity.email)
        assertEquals(1, antigravity.accounts.size)
        val agAccount = antigravity.accounts[0]
        assertTrue(agAccount.id.startsWith("acc_"))
        assertEquals(false, agAccount.active)
        assertEquals(false, agAccount.env)
        assertNotNull(agAccount.created_at)

        // codex: two accounts, the first active, the second parked.
        val codex = data.codex!!
        assertEquals(2, codex.accounts.size)
        val primary = codex.accounts[0]
        assertEquals("acc_1feed2ee3aa44bb5", primary.id)
        assertEquals(true, primary.active)
        val parked = codex.accounts[1]
        assertEquals("second@example.com", parked.email)
        assertEquals(false, parked.active)

        // claude: the synthetic env account is active and env-flagged; the
        // file account next to it reports inactive because the env token
        // owns credential resolution while the variable is set.
        val claude = data.claude!!
        assertEquals(2, claude.accounts.size)
        val env = claude.accounts[0]
        assertEquals("env", env.id)
        assertEquals(true, env.active)
        assertEquals(true, env.env)
        val fileAccount = claude.accounts[1]
        assertEquals(true, fileAccount.id.startsWith("acc_"))
        assertEquals(false, fileAccount.active)
        assertEquals(false, fileAccount.env)
    }

    @Test
    fun activeAndEnvAreExplicitEvenWhenFalse() {
        val data = statusAdapter.fromJson(fixture("status.json"))!!
        val parked = data.codex!!.accounts[1]
        // protojson omits implicit-presence false; the shared proto makes
        // these explicit so "not in use" is a real value on the wire.
        assertEquals(false, parked.active)
        assertEquals(false, parked.env)
    }

    @Test
    fun decodesLegacyPayloadWithoutAccounts() {
        val data = statusAdapter.fromJson(fixture("status_no_accounts.json"))!!
        // Old payloads keep decoding: the accounts list is simply absent.
        val codex = data.codex!!
        assertTrue(codex.logged_in)
        assertEquals("adelodunpeter24@gmail.com", codex.email)
        assertEquals(0, codex.accounts.size)
        val claude = data.claude!!
        assertTrue(claude.logged_in)
        assertEquals(0, claude.accounts.size)
    }

    @Test
    fun decodesCallbackWithAccountIdAndActiveFalse() {
        val data = callbackAdapter.fromJson(fixture("callback.json"))!!
        assertEquals("codex", data.provider)
        assertEquals("user@example.com", data.user_email)
        // account_id plus account_active=false is the "second login did not
        // steal active" payload the client uses to prompt for a Use pick.
        assertEquals("acc_1feed2ee3aa44bb5", data.account_id)
        assertEquals(false, data.account_active)
    }
}
