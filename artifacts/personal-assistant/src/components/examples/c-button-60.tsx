import {
  RiGithubFill,
  RiGoogleFill,
} from '@remixicon/react';
import { Button } from '@workspace/askolo-design-system/components/ui/button';

export function CButton60SocialAuthButtons({
  intent,
  onGoogleClick,
  onGithubClick,
}: {
  intent: 'signin' | 'signup';
  onGoogleClick: () => void;
  onGithubClick: () => void;
}) {
  const action = intent === 'signup' ? 'Sign up' : 'Login';

  return (
    <div
      role="group"
      aria-label={intent === 'signup' ? 'Account sign-up providers' : 'Account sign-in providers'}
      className="flex flex-wrap justify-center gap-2"
    >
      <Button
        type="button"
        aria-label={`${action} with Google`}
        className="min-h-11 min-w-11 shrink-0"
        size="icon"
        variant="outline"
        onClick={onGoogleClick}
        data-testid="button-google-login"
      >
        <RiGoogleFill
          aria-hidden="true"
          className="text-[#DB4437] dark:text-primary"
          size={16}
        />
      </Button>
      <Button
        type="button"
        aria-label={`${action} with GitHub`}
        className="min-h-11 min-w-11 shrink-0"
        size="icon"
        variant="outline"
        onClick={onGithubClick}
        data-testid="button-github-login"
      >
        <RiGithubFill
          aria-hidden="true"
          className="text-black dark:text-primary"
          size={16}
        />
      </Button>
    </div>
  );
}