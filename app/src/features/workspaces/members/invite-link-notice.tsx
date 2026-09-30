import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { TruncatedText } from "@/components/truncated-text";
import type { CreatedInvite } from "@/features/workspaces/types";
import { sentence } from "@/utils/sentence";

// Shown when an invite couldn't be emailed (e.g. Resend isn't configured), so
// the inviter can pass the link or code along themselves.
export function InviteLinkNotice({ invite }: { invite: CreatedInvite }) {
  const [copied, setCopied] = useState(false);

  const copyLink = async () => {
    await navigator.clipboard.writeText(invite.link);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Alert>
      <AlertTitle>Share this invite with {invite.email}</AlertTitle>
      <AlertDescription className="flex flex-col gap-3">
        <span>{sentence(invite.emailError ?? "the invite email couldn't be sent.")}</span>
        <div className="flex items-center gap-2">
          <TruncatedText
            text={invite.link}
            className="min-w-0 flex-1 rounded-md border bg-muted px-2 py-1.5 font-mono text-xs"
          />
          <Button size="sm" variant="outline" onClick={copyLink}>
            {copied ? <Check /> : <Copy />}
            {copied ? "Copied" : "Copy link"}
          </Button>
        </div>
        <span>
          Or they can enter the code{" "}
          <code className="font-mono font-medium text-foreground">{invite.code}</code>{" "}
          after signing in. It works once and expires in 7 days.
        </span>
      </AlertDescription>
    </Alert>
  );
}
