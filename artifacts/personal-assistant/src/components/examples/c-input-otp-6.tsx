import {
  Field,
  FieldDescription,
  FieldLabel,
} from "@/components/ui/field";
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSeparator,
  InputOTPSlot,
} from "@/components/ui/input-otp";
import { REGEXP_ONLY_DIGITS } from "input-otp";

type CInputOtp6Props = {
  id: string;
  label: string;
  description?: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  required?: boolean;
  ariaDescribedBy?: string;
  ariaInvalid?: boolean;
  testId?: string;
};

export function CInputOtp6({
  id,
  label,
  description,
  value,
  onChange,
  disabled = false,
  required = true,
  ariaDescribedBy,
  ariaInvalid = false,
  testId,
}: CInputOtp6Props) {
  const describedBy = [
    description ? `${id}-description` : undefined,
    ariaDescribedBy,
  ]
    .filter(Boolean)
    .join(" ") || undefined;
  const slotClassName = "h-9 w-9 sm:h-10 sm:w-10";

  return (
    <Field className="gap-2">
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {description && (
        <FieldDescription id={`${id}-description`}>
          {description}
        </FieldDescription>
      )}
      <InputOTP
        id={id}
        maxLength={6}
        value={value}
        onChange={(nextValue) => onChange(nextValue.replace(/\D/g, "").slice(0, 6))}
        inputMode="numeric"
        autoComplete="one-time-code"
        pattern={REGEXP_ONLY_DIGITS}
        required={required}
        disabled={disabled}
        aria-describedby={describedBy}
        aria-invalid={ariaInvalid}
        data-testid={testId}
      >
        <InputOTPGroup>
          {[0, 1, 2].map((index) => (
            <InputOTPSlot
              key={index}
              index={index}
              className={slotClassName}
              aria-invalid={ariaInvalid}
            />
          ))}
        </InputOTPGroup>
        <InputOTPSeparator />
        <InputOTPGroup>
          {[3, 4, 5].map((index) => (
            <InputOTPSlot
              key={index}
              index={index}
              className={slotClassName}
              aria-invalid={ariaInvalid}
            />
          ))}
        </InputOTPGroup>
      </InputOTP>
    </Field>
  );
}