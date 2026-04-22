package expo.modules.notificationcapture

import android.app.Notification
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import org.json.JSONObject

/**
 * NotificationListenerService implementation that captures cross-app
 * notifications when the user has granted Notification Access in Android
 * system settings.
 *
 * Lifecycle: bound by the OS at boot whenever the user has the access
 * granted. We *do not* start it manually — the system manages it.
 *
 * Filtering happens in two places:
 *   1. Master switch: NotificationStore.isEnabled() controls whether we
 *      record anything at all (so the user can pause without revoking
 *      Notification Access).
 *   2. App allowlist: NotificationStore.getAllowlist() restricts capture
 *      to apps the user explicitly opted into. If the allowlist is empty
 *      we fall through to nothing being captured (safe default).
 *
 * Captured payload mirrors the backend's notifications.EventInput JSON
 * shape so the JS layer can forward it without transformation.
 */
class NotificationCaptureService : NotificationListenerService() {

    private val store: NotificationStore by lazy { NotificationStore(applicationContext) }

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        if (!store.isEnabled()) return

        val pkg = sbn.packageName ?: return
        val allowlist = store.getAllowlist()
        if (allowlist.isEmpty() || !allowlist.contains(pkg)) return

        val notification = sbn.notification ?: return
        val extras = notification.extras

        val title = extras?.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        val text = extractText(extras)
        val category = notification.category.orEmpty()
        val appLabel = resolveAppLabel(pkg)

        val event = JSONObject().apply {
            put("app_package", pkg)
            put("app_label", appLabel)
            put("title", title)
            put("content", text)
            put("category", category)
            put("captured_at", android.text.format.DateFormat.format(
                "yyyy-MM-dd'T'HH:mm:ss.SSS'Z'",
                java.util.Calendar.getInstance(java.util.TimeZone.getTimeZone("UTC")).apply {
                    timeInMillis = sbn.postTime
                }
            ).toString())
        }

        store.bufferEvent(event)
    }

    /**
     * Extract human-readable text from the notification extras bundle.
     * Tries EXTRA_BIG_TEXT first (full body for messaging apps) and falls
     * back to EXTRA_TEXT (preview line). Joining EXTRA_TEXT_LINES handles
     * inbox-style notifications (e.g. Gmail summaries).
     */
    private fun extractText(extras: android.os.Bundle?): String {
        if (extras == null) return ""
        val big = extras.getCharSequence(Notification.EXTRA_BIG_TEXT)?.toString()
        if (!big.isNullOrEmpty()) return big

        val short = extras.getCharSequence(Notification.EXTRA_TEXT)?.toString()
        if (!short.isNullOrEmpty()) return short

        val lines = extras.getCharSequenceArray(Notification.EXTRA_TEXT_LINES)
        if (lines != null && lines.isNotEmpty()) {
            return lines.joinToString(separator = "\n") { it.toString() }
        }
        return ""
    }

    /**
     * Translate a package name to a user-readable app label using the
     * PackageManager. Falls back to the raw package on failure (better
     * than a blank field in the rollup).
     */
    private fun resolveAppLabel(pkg: String): String {
        return try {
            val pm = packageManager
            val info = pm.getApplicationInfo(pkg, 0)
            pm.getApplicationLabel(info).toString()
        } catch (_: Exception) {
            pkg
        }
    }
}
