import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group";

type CInputGroup9CurrencyInputProps = {
  id: string;
  value: string;
  onValueChange: (value: string) => void;
  placeholder?: string;
  ariaLabel?: string;
  ariaDescribedBy?: string;
  ariaInvalid?: boolean;
  allowNegative?: boolean;
  disabled?: boolean;
  testId?: string;
};

export function CInputGroup9CurrencyInput({
  id,
  value,
  onValueChange,
  placeholder = "0.00",
  ariaLabel,
  ariaDescribedBy,
  ariaInvalid = false,
  allowNegative = false,
  disabled = false,
  testId,
}: CInputGroup9CurrencyInputProps) {
  return (
    <InputGroup>
      <InputGroupAddon aria-hidden="true">
        <InputGroupText>$</InputGroupText>
      </InputGroupAddon>
      <InputGroupInput
        id={id}
        type="text"
        inputMode={allowNegative ? "text" : "decimal"}
        autoComplete="off"
        spellCheck={false}
        value={value}
        onChange={(event) => onValueChange(event.target.value)}
        placeholder={placeholder}
        aria-label={ariaLabel}
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid}
        disabled={disabled}
        data-testid={testId}
      />
      <InputGroupAddon align="inline-end" aria-hidden="true">
        <InputGroupText>USD</InputGroupText>
      </InputGroupAddon>
    </InputGroup>
  );
}