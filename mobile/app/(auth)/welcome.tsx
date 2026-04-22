import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, KeyboardAvoidingView, Platform, View } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { OTPInput } from "@/components/ui/OTPInput";
import { Text } from "@/components/ui/Text";
import { useAuthSession } from "@/providers/AuthSessionProvider";

const RESEND_COOLDOWN_SECONDS = 30;

export default function WelcomeScreen() {
  const router = useRouter();
  const params = useLocalSearchParams<{ email?: string; invite_token?: string }>();
  const { login, verifyCode } = useAuthSession();

  // When the user reaches this screen via the invite landing flow, both the
  // recipient email and the invite_token are passed in. The email is locked
  // to the recipient (so a logged-out user can't accept on someone else's
  // behalf) and the token rides through magic-link → verify, which auto-
  // accepts the invite once the JWT is issued.
  const initialEmail = typeof params.email === "string" ? params.email : "";
  const inviteToken = typeof params.invite_token === "string" ? params.invite_token : undefined;
  const isInviteFlow = !!inviteToken;

  const [step, setStep] = useState<"email" | "code">("email");
  const [email, setEmail] = useState(initialEmail);
  const [code, setCode] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  const startCooldown = useCallback(() => {
    setCooldown(RESEND_COOLDOWN_SECONDS);
    if (timerRef.current) clearInterval(timerRef.current);
    timerRef.current = setInterval(() => {
      setCooldown((prev) => {
        if (prev <= 1) {
          if (timerRef.current) clearInterval(timerRef.current);
          timerRef.current = null;
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
  }, []);

  const sendCode = async () => {
    if (!email.trim()) {
      Alert.alert("Email required", "Enter your email address to continue.");
      return;
    }
    if (cooldown > 0) return;
    setSubmitting(true);
    try {
      await login(email.trim(), inviteToken);
      setStep("code");
      startCooldown();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to send verification code";
      Alert.alert("Unable to send code", message);
    } finally {
      setSubmitting(false);
    }
  };

  const confirmCode = async () => {
    if (!code.trim()) {
      Alert.alert("Code required", "Enter the 6-digit verification code.");
      return;
    }
    setSubmitting(true);
    try {
      const { inviteError } = await verifyCode(email.trim(), code.trim(), inviteToken);
      if (inviteError) {
        // The login itself succeeded but the invite couldn't be applied —
        // surface it so the user understands why they're not in the team
        // they expected to land in.
        Alert.alert("Invite could not be applied", inviteError);
      }
      router.replace("/(app)");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Invalid code";
      Alert.alert("Verification failed", message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <KeyboardAvoidingView
      className="flex-1 items-center justify-center bg-background px-5"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>
            {isInviteFlow
              ? step === "email"
                ? "Accept your invite"
                : "Verify to join"
              : step === "email"
                ? "Sign in with magic link"
                : "Enter verification code"}
          </CardTitle>
          <CardDescription>
            {isInviteFlow
              ? step === "email"
                ? "Sign in with the invited email below to join the team."
                : `We sent a code to ${email}. Enter it to join the team.`
              : step === "email"
                ? "Use your email to receive a secure one-time code."
                : `We sent a code to ${email}. Enter it below to continue.`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {step === "email" ? (
            <View className="gap-4">
              <Input
                label="Email"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
                returnKeyType="go"
                value={email}
                onChangeText={setEmail}
                onSubmitEditing={sendCode}
                placeholder="you@example.com"
                editable={!isInviteFlow}
              />
              <Button loading={submitting} onPress={sendCode}>
                {isInviteFlow ? "Send code to join" : "Send code"}
              </Button>
            </View>
          ) : (
            <View className="gap-4">
              <OTPInput
                label="Code"
                value={code}
                onChange={setCode}
                onComplete={confirmCode}
              />
              <Button loading={submitting} onPress={confirmCode}>
                Verify
              </Button>
              <Button
                variant="ghost"
                size="sm"
                disabled={cooldown > 0}
                onPress={sendCode}
              >
                {cooldown > 0 ? `Resend code (${cooldown}s)` : "Resend code"}
              </Button>
            </View>
          )}
        </CardContent>
      </Card>
      <Text variant="muted" className="mt-4 text-center">
        Sign in with a secure one-time code. No password needed.
      </Text>
    </KeyboardAvoidingView>
  );
}
