package expo.modules.notificationcapture

import android.content.ComponentName
import android.content.Intent
import android.provider.Settings
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * Expo module bridge between JS and the NotificationListenerService.
 *
 * Exposed surface:
 *   - hasPermission(): Boolean — is Notification Access granted?
 *   - openSettings(): Void — deep-link the user to the system settings page
 *   - setEnabled(enabled: Boolean): Void — master capture switch
 *   - isEnabled(): Boolean — read the master switch
 *   - setAllowlist(packages: [String]): Void — set monitored apps
 *   - getAllowlist(): [String] — read monitored apps
 *   - drainBuffer(): String — fetch and clear pending events (JSON array)
 *   - requeueEvents(json: String): Void — prepend events back to the
 *       buffer when an upload fails, so we never lose data on flaky
 *       networks. JSON shape must match what drainBuffer() returns.
 *   - bufferSize(): Int — count pending events without draining
 *   - lastSyncAt(): Number — ms-since-epoch of the last flush, or 0
 *   - markSynced(timestampMs: Number): Void — record a successful flush
 *
 * Everything is synchronous because the underlying NotificationStore
 * uses SharedPreferences, which is fast and main-thread-safe at this
 * volume. If we ever migrate to SQLite we'll switch to AsyncFunction.
 */
class NotificationCaptureModule : Module() {

    private val store: NotificationStore by lazy {
        NotificationStore(appContext.reactContext!!.applicationContext)
    }

    override fun definition() = ModuleDefinition {
        Name("NotificationCapture")

        Function("hasPermission") {
            val ctx = appContext.reactContext ?: return@Function false
            val enabled = Settings.Secure.getString(
                ctx.contentResolver,
                "enabled_notification_listeners"
            ) ?: return@Function false
            val expected = ComponentName(ctx, NotificationCaptureService::class.java).flattenToString()
            enabled.split(":").any { it == expected }
        }

        Function("openSettings") {
            val ctx = appContext.reactContext ?: return@Function
            val intent = Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            ctx.startActivity(intent)
        }

        Function("isEnabled") { store.isEnabled() }

        Function("setEnabled") { enabled: Boolean -> store.setEnabled(enabled) }

        Function("getAllowlist") { store.getAllowlist().toList() }

        Function("setAllowlist") { packages: List<String> ->
            store.setAllowlist(packages.toSet())
        }

        Function("drainBuffer") { store.drainBuffer() }

        Function("requeueEvents") { eventsJson: String -> store.requeueEvents(eventsJson) }

        Function("bufferSize") { store.bufferSize() }

        Function("lastSyncAt") { store.getLastSyncAtMs().toDouble() }

        Function("markSynced") { timestampMs: Double ->
            store.setLastSyncAtMs(timestampMs.toLong())
        }
    }
}
