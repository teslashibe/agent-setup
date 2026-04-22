package expo.modules.notificationcapture

import android.content.Context
import android.content.SharedPreferences
import org.json.JSONArray
import org.json.JSONObject

/**
 * Tiny persistence layer for captured notifications and runtime preferences.
 *
 * Why SharedPreferences instead of Room/SQLite? The total volume is small
 * (≤ ~5 minutes of notifications between flushes, ~50 events worst case),
 * and SharedPreferences gives us atomic file writes and zero-cost startup.
 * If the buffer ever grows past a few hundred events we'll move to a
 * proper SQLite cache, but for V1 this keeps the module dependency-free.
 *
 * Stored data:
 *   - "buffer"          → JSON array of pending notification events
 *   - "allowlist"       → JSON array of app package strings
 *   - "is_enabled"      → boolean master switch
 *   - "last_sync_at_ms" → timestamp of the last successful flush
 */
class NotificationStore(context: Context) {

    private val prefs: SharedPreferences = context.applicationContext.getSharedPreferences(
        PREFS_NAME,
        Context.MODE_PRIVATE
    )

    fun isEnabled(): Boolean = prefs.getBoolean(KEY_ENABLED, false)

    fun setEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    fun getAllowlist(): Set<String> {
        val raw = prefs.getString(KEY_ALLOWLIST, "[]") ?: "[]"
        return try {
            val arr = JSONArray(raw)
            (0 until arr.length()).map { arr.getString(it) }.toSet()
        } catch (_: Exception) {
            emptySet()
        }
    }

    fun setAllowlist(packages: Collection<String>) {
        val arr = JSONArray()
        packages.forEach { arr.put(it) }
        prefs.edit().putString(KEY_ALLOWLIST, arr.toString()).apply()
    }

    fun getLastSyncAtMs(): Long = prefs.getLong(KEY_LAST_SYNC, 0L)

    fun setLastSyncAtMs(ts: Long) {
        prefs.edit().putLong(KEY_LAST_SYNC, ts).apply()
    }

    /**
     * Append a single notification event to the on-disk buffer. Called by
     * NotificationCaptureService on every onNotificationPosted call. Bounded
     * to MAX_BUFFER_SIZE entries; the oldest are dropped first so we don't
     * grow without bound if the user revokes network access.
     */
    @Synchronized
    fun bufferEvent(event: JSONObject) {
        val current = readBuffer()
        current.put(event)
        while (current.length() > MAX_BUFFER_SIZE) {
            current.remove(0)
        }
        prefs.edit().putString(KEY_BUFFER, current.toString()).apply()
    }

    /**
     * Read and clear the in-memory buffer atomically. Returns the JSON
     * array as a string so the JS side can pass it through unchanged to
     * the API (no parse/re-stringify round-trip).
     */
    @Synchronized
    fun drainBuffer(): String {
        val current = readBuffer()
        prefs.edit().putString(KEY_BUFFER, "[]").apply()
        return current.toString()
    }

    @Synchronized
    fun bufferSize(): Int = readBuffer().length()

    /**
     * Prepend a batch of events back to the buffer. Used by the JS layer
     * to recover from a failed upload without losing data. Older events
     * are dropped first if the combined buffer would exceed MAX_BUFFER_SIZE
     * — we always keep the freshest data because that's what the rollup
     * cares about most.
     */
    @Synchronized
    fun requeueEvents(eventsJson: String) {
        val incoming = try {
            JSONArray(eventsJson)
        } catch (_: Exception) {
            return
        }
        if (incoming.length() == 0) return

        val current = readBuffer()
        val combined = JSONArray()
        for (i in 0 until incoming.length()) combined.put(incoming.get(i))
        for (i in 0 until current.length()) combined.put(current.get(i))

        while (combined.length() > MAX_BUFFER_SIZE) {
            combined.remove(0)
        }
        prefs.edit().putString(KEY_BUFFER, combined.toString()).apply()
    }

    private fun readBuffer(): JSONArray {
        val raw = prefs.getString(KEY_BUFFER, "[]") ?: "[]"
        return try {
            JSONArray(raw)
        } catch (_: Exception) {
            JSONArray()
        }
    }

    companion object {
        private const val PREFS_NAME = "expo.modules.notificationcapture.prefs"
        private const val KEY_ENABLED = "is_enabled"
        private const val KEY_ALLOWLIST = "allowlist"
        private const val KEY_BUFFER = "buffer"
        private const val KEY_LAST_SYNC = "last_sync_at_ms"
        private const val MAX_BUFFER_SIZE = 500
    }
}
