import { Badge } from "@/components/ui/Badge";
import type { TeamRole } from "@/services/teams";

// roleVariant + roleLabel were duplicated across teams/[id].tsx, settings.tsx,
// and any place that shows a member's tier. Moving them into one component
// gives every screen the same colour, label, and accessibility name without
// the cut-and-paste table that the audit flagged in M3.
const roleLabel: Record<TeamRole, string> = {
  owner: "Owner",
  admin: "Admin",
  member: "Member",
};

const roleVariant: Record<TeamRole, "default" | "secondary" | "outline"> = {
  owner: "default",
  admin: "secondary",
  member: "outline",
};

export function RoleBadge({
  role,
  className,
}: {
  role: TeamRole;
  className?: string;
}) {
  return (
    <Badge variant={roleVariant[role]} className={className}>
      {roleLabel[role]}
    </Badge>
  );
}
