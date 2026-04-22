import { ReactNode } from "react";
import { Modal, Pressable, View } from "react-native";

import { Button } from "@/components/ui/Button";
import { Text } from "@/components/ui/Text";
import { cn } from "@/lib/utils";

// Dialog is a theme-aware confirm sheet. Use it instead of Alert.alert for
// destructive flows (leave team, delete team, revoke invite, remove member)
// because Alert.alert renders the host OS alert which:
//   - has zero theming on iOS/Android (light vs dark looks wrong),
//   - doesn't render on react-native-web at all,
//   - can't host arbitrary children (e.g. an embedded RoleBadge in the body).
//
// Keep the API surface tiny: open/close + title + description + slots for the
// confirm/cancel buttons. Anything fancier should be its own component.
type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children?: ReactNode;
  /** Label for the primary action; pair with onConfirm. */
  confirmLabel?: string;
  /** Variant for the primary action — destructive flows pass "destructive". */
  confirmVariant?: "default" | "destructive";
  /** Loading state for the primary action; disables both buttons. */
  confirmLoading?: boolean;
  onConfirm?: () => void | Promise<void>;
  cancelLabel?: string;
};

export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  confirmLabel = "Confirm",
  confirmVariant = "default",
  confirmLoading = false,
  onConfirm,
  cancelLabel = "Cancel",
}: DialogProps) {
  const close = () => {
    if (confirmLoading) return;
    onOpenChange(false);
  };
  return (
    <Modal
      transparent
      animationType="fade"
      visible={open}
      onRequestClose={close}
      statusBarTranslucent
    >
      <Pressable
        accessibilityLabel="Dismiss dialog"
        className="flex-1 items-center justify-center bg-black/60 px-6"
        onPress={close}
      >
        {/* Inner Pressable swallows taps so clicking inside the card doesn't
            close the dialog. */}
        <Pressable
          onPress={() => undefined}
          className={cn(
            "w-full max-w-sm rounded-2xl border border-border bg-card p-5",
          )}
        >
          <Text variant="h3" className="mb-1">
            {title}
          </Text>
          {description ? (
            <Text variant="muted" className="mb-3">
              {description}
            </Text>
          ) : null}
          {children}
          <View className="mt-4 flex-row justify-end gap-2">
            <Button variant="ghost" onPress={close} disabled={confirmLoading}>
              {cancelLabel}
            </Button>
            {onConfirm ? (
              <Button
                variant={confirmVariant}
                loading={confirmLoading}
                onPress={onConfirm}
              >
                {confirmLabel}
              </Button>
            ) : null}
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}
