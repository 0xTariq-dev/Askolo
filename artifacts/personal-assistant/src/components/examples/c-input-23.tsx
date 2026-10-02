import { useMemo, useState } from "react";
import { Check, Eye, EyeOff, X } from "lucide-react";
import {
  Field,
  FieldDescription,
  FieldLabel,
} from "@/components/ui/field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";

type PasswordRequirement = {
  met: boolean;
  text: string;
};

type CInput23PasswordFieldsProps = {
  idPrefix: string;
  password: string;
  confirmation: string;
  onPasswordChange: (value: string) => void;
  onConfirmationChange: (value: string) => void;
  passwordLabel?: string;
  confirmationLabel?: string;
  passwordPlaceholder?: string;
  confirmationPlaceholder?: string;
  errorDescribedBy?: string;
};

function getPasswordRequirements(password: string): PasswordRequirement[] {
  return [
    { met: password.length >= 8, text: "At least 8 characters" },
    { met: /[0-9]/.test(password), text: "At least 1 number" },
    { met: /[a-z]/.test(password), text: "At least 1 lowercase letter" },
    { met: /[A-Z]/.test(password), text: "At least 1 uppercase letter" },
    {
      met: /[!@#$%^&*(),.?":{}|<>]/.test(password),
      text: "At least 1 special character",
    },
  ];
}

function getStrengthText(score: number): string {
  if (score === 0) return "Enter a password";
  if (score <= 2) return "Weak security";
  if (score <= 4) return "Medium security";
  return "Strong security";
}

export function CInput23PasswordFields({
  idPrefix,
  password,
  confirmation,
  onPasswordChange,
  onConfirmationChange,
  passwordLabel = "Password",
  confirmationLabel = "Confirm password",
  passwordPlaceholder = "Create a strong password",
  confirmationPlaceholder = "Re-enter your password",
  errorDescribedBy,
}: CInput23PasswordFieldsProps) {
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [confirmationVisible, setConfirmationVisible] = useState(false);
  const passwordId = `${idPrefix}-password`;
  const confirmationId = `${idPrefix}-confirm-password`;
  const requirementsId = `${passwordId}-requirements`;
  const strengthId = `${passwordId}-strength`;
  const matchId = `${confirmationId}-match`;
  const requirements = useMemo(
    () => getPasswordRequirements(password),
    [password],
  );
  const strengthScore = requirements.filter((requirement) => requirement.met).length;
  const strengthText = getStrengthText(strengthScore);
  const passwordMismatch = confirmation.length > 0 && password !== confirmation;
  const confirmationDescription = passwordMismatch
    ? "Passwords do not match."
    : confirmation.length > 0
      ? "Passwords match."
      : undefined;

  return (
    <div className="space-y-4">
      <Field>
        <FieldLabel htmlFor={passwordId}>{passwordLabel}</FieldLabel>
        <InputGroup>
          <InputGroupInput
            id={passwordId}
            type={passwordVisible ? "text" : "password"}
            autoComplete="new-password"
            placeholder={passwordPlaceholder}
            value={password}
            onChange={(event) => onPasswordChange(event.target.value)}
            aria-describedby={`${strengthId} ${requirementsId}${errorDescribedBy ? ` ${errorDescribedBy}` : ""}`}
            required
            data-testid={`input-${idPrefix}-password`}
          />
          <InputGroupAddon align="inline-end">
            <InputGroupButton
              type="button"
              size="icon-sm"
              aria-label={passwordVisible ? "Hide password" : "Show password"}
              aria-controls={passwordId}
              aria-pressed={passwordVisible}
              onClick={() => setPasswordVisible((visible) => !visible)}
              data-testid={`toggle-${idPrefix}-password`}
            >
              {passwordVisible ? (
                <EyeOff aria-hidden="true" />
              ) : (
                <Eye aria-hidden="true" />
              )}
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
      </Field>

      <div>
        <div
          role="progressbar"
          aria-label="Password strength"
          aria-valuemin={0}
          aria-valuemax={5}
          aria-valuenow={strengthScore}
          aria-valuetext={strengthText}
          className="mb-3 flex gap-1"
        >
          {[0, 1, 2, 3, 4].map((segment) => (
            <div
              key={segment}
              aria-hidden="true"
              className={`h-1 flex-1 rounded-full ${
                segment < strengthScore ? "bg-primary" : "bg-muted"
              }`}
            />
          ))}
        </div>
        <p id={strengthId} className="text-sm font-medium" aria-live="polite">
          {strengthText} · {strengthScore}/5 requirements met
        </p>
        <ul id={requirementsId} aria-label="Password requirements" className="mt-2 space-y-1.5">
          {requirements.map((requirement) => (
            <li
              className="flex items-center gap-2 text-xs"
              key={requirement.text}
            >
              {requirement.met ? (
                <Check
                  aria-hidden="true"
                  className="h-3.5 w-3.5 shrink-0 text-primary"
                />
              ) : (
                <X
                  aria-hidden="true"
                  className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
                />
              )}
              <span>
                {requirement.text}
                <span className="sr-only">
                  {requirement.met ? " — requirement met" : " — requirement not met"}
                </span>
              </span>
            </li>
          ))}
        </ul>
      </div>

      <Field>
        <FieldLabel htmlFor={confirmationId}>{confirmationLabel}</FieldLabel>
        <InputGroup>
          <InputGroupInput
            id={confirmationId}
            type={confirmationVisible ? "text" : "password"}
            autoComplete="new-password"
            placeholder={confirmationPlaceholder}
            value={confirmation}
            onChange={(event) => onConfirmationChange(event.target.value)}
            aria-invalid={passwordMismatch}
            aria-describedby={confirmationDescription ? matchId : undefined}
            required
            data-testid={`input-${idPrefix}-confirm-password`}
          />
          <InputGroupAddon align="inline-end">
            <InputGroupButton
              type="button"
              size="icon-sm"
              aria-label={confirmationVisible ? "Hide confirmation" : "Show confirmation"}
              aria-controls={confirmationId}
              aria-pressed={confirmationVisible}
              onClick={() => setConfirmationVisible((visible) => !visible)}
              data-testid={`toggle-${idPrefix}-confirm-password`}
            >
              {confirmationVisible ? (
                <EyeOff aria-hidden="true" />
              ) : (
                <Eye aria-hidden="true" />
              )}
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
        {confirmationDescription && (
          <FieldDescription
            id={matchId}
            aria-live="polite"
            className={passwordMismatch ? "text-destructive" : "text-primary"}
          >
            {confirmationDescription}
          </FieldDescription>
        )}
      </Field>
    </div>
  );
}