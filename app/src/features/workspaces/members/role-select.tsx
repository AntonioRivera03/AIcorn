import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { Role } from "@/features/workspaces/types";
import { ROLE_LABELS } from "@/features/workspaces/members/role-labels";

type Props = {
  value: Role;
  roles: Role[];
  onChange: (role: Role) => void;
  disabled?: boolean;
  id?: string;
};

export function RoleSelect({ value, roles, onChange, disabled, id }: Props) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as Role)} disabled={disabled}>
      <SelectTrigger id={id} size="sm" className="w-28">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {roles.map((role) => (
          <SelectItem key={role} value={role}>
            {ROLE_LABELS[role]}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
