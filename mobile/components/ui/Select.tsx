import { ReactNode, useState } from "react";
import { Modal, Pressable, View } from "react-native";
import { Check, ChevronDown } from "lucide-react-native";

import { Text } from "@/components/ui/Text";
import { cn } from "@/lib/utils";

// Option<T> stays generic so callers can pick any value type — string roles,
// team IDs, etc. label is optional so a Membership entry can render a custom
// row via `children` without surrendering the value type.
export type SelectOption<T> = {
  value: T;
  label: string;
  description?: string;
  disabled?: boolean;
};

type SelectProps<T> = {
  value: T;
  options: SelectOption<T>[];
  onValueChange: (value: T) => void;
  placeholder?: string;
  /** Wraps the trigger content; falls back to the matched option label. */
  renderTrigger?: (selected: SelectOption<T> | undefined) => ReactNode;
  className?: string;
  disabled?: boolean;
};

// Select replaces the row of <Pressable><Badge /></Pressable> we were using
// for inline pickers. Single-value, modal-backed, theme-aware, and keyboard
// dismissible by tapping outside. Use this for: invite role, member role
// changes, team-switching dropdown.
export function Select<T extends string | number>({
  value,
  options,
  onValueChange,
  placeholder = "Select…",
  renderTrigger,
  className,
  disabled,
}: SelectProps<T>) {
  const [open, setOpen] = useState(false);
  const selected = options.find((o) => o.value === value);

  return (
    <>
      <Pressable
        accessibilityRole="combobox"
        accessibilityState={{ expanded: open, disabled }}
        disabled={disabled}
        className={cn(
          // min-h (not fixed h) so callers using renderTrigger with multi-line
          // labels (e.g. team name + subtitle) don't get content clipped.
          "min-h-11 flex-row items-center justify-between rounded-lg border border-border bg-card px-3 py-2",
          disabled ? "opacity-50" : "",
          className,
        )}
        onPress={() => setOpen(true)}
      >
        {renderTrigger ? (
          renderTrigger(selected)
        ) : (
          <Text variant="p" numberOfLines={1} className="flex-1">
            {selected?.label ?? placeholder}
          </Text>
        )}
        <ChevronDown size={16} color="#9AA4B2" />
      </Pressable>

      <Modal
        transparent
        animationType="fade"
        visible={open}
        statusBarTranslucent
        onRequestClose={() => setOpen(false)}
      >
        <Pressable
          accessibilityLabel="Dismiss picker"
          className="flex-1 items-center justify-end bg-black/60 px-4 pb-8"
          onPress={() => setOpen(false)}
        >
          <Pressable
            onPress={() => undefined}
            className="w-full max-w-sm rounded-2xl border border-border bg-card p-2"
          >
            {options.map((opt) => {
              const active = opt.value === value;
              return (
                <Pressable
                  key={String(opt.value)}
                  disabled={opt.disabled}
                  className={cn(
                    "flex-row items-center justify-between rounded-lg px-3 py-3 active:bg-secondary",
                    opt.disabled ? "opacity-50" : "",
                  )}
                  onPress={() => {
                    onValueChange(opt.value);
                    setOpen(false);
                  }}
                >
                  <View className="flex-1 pr-3">
                    <Text variant="p">{opt.label}</Text>
                    {opt.description ? (
                      <Text variant="muted">{opt.description}</Text>
                    ) : null}
                  </View>
                  {active ? <Check size={16} color="#00D4AA" /> : null}
                </Pressable>
              );
            })}
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}
