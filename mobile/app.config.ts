import type { ConfigContext, ExpoConfig } from "expo/config";

const semverToBuildNumber = (version: string): string => {
  const match = version.match(/^(\d+)\.(\d+)\.(\d+)/);
  if (!match) {
    return "1";
  }
  const major = Number(match[1]);
  const minor = Number(match[2]);
  const patch = Number(match[3]);
  return String(major * 10000 + minor * 100 + patch);
};

export default ({ config }: ConfigContext): ExpoConfig => {
  const version = process.env.APP_VERSION ?? "1.0.0";

  return {
    ...config,
    name: "Claude Agent Go",
    slug: "claude-agent-go",
    owner: "teslashibe",
    version,
    scheme: "agentapp",
    userInterfaceStyle: "dark",
    orientation: "portrait",
    ios: {
      bundleIdentifier: "com.teslashibe.agentapp",
      buildNumber: semverToBuildNumber(version),
      supportsTablet: true
    },
    android: {
      package: "com.teslashibe.agentapp"
    },
    plugins: [
      "expo-router",
      "expo-web-browser",
      "expo-secure-store",
      [
        "expo-image-picker",
        {
          // Permission strings shown by iOS when the chat composer
          // opens the photo library to attach an image. We only need
          // library access — no camera, no microphone — because the
          // attachment flow is "pick a photo you already have".
          photosPermission:
            "Allow $(PRODUCT_NAME) to access your photos so you can attach images to your agent's posts.",
          cameraPermission: false,
          microphonePermission: false
        }
      ]
    ],
    experiments: {
      typedRoutes: true
    },
    extra: {
      eas: {
        projectId: "88f1130e-f72f-4f93-a60f-fc25e556c06b"
      }
    }
  };
};
