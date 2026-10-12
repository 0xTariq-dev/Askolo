import { useState } from 'react';
import { ArrowUp, AudioLines, Mic, PhoneCall } from 'lucide-react';
import { useLocation } from 'wouter';
import { Button } from '@/components/ui/button';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { useAssistantState } from '@/contexts/assistant-context';
import { useLocale } from '@/contexts/locale-context';

export function HomeAgentComposer() {
  const [prompt, setPrompt] = useState('');
  const [, setLocation] = useLocation();
  const { openWithDraft } = useAssistantState();
  const { locale } = useLocale();
  const hasPrompt = Boolean(prompt.trim());

  const continueInAgent = () => {
    openWithDraft(prompt.trim());
    setLocation('/agent');
  };

  const submit = () => {
    const message = prompt.trim() || (locale === 'ar'
      ? 'ساعدني في التفكير في يومي.'
      : 'Help me think through today.');
    openWithDraft(message);
    setLocation('/agent');
  };

  return (
    <form
      className="w-full"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <label className="sr-only" htmlFor="home-agent-prompt">
        {locale === 'ar' ? 'ماذا تريد أن تسأل الوكيل؟' : 'What would you like to ask Agent?'}
      </label>
      <InputGroup className="agent-composer-input mx-auto h-14 w-full max-w-[780px] rounded-full border border-border/60 bg-background p-1.5 shadow-sm dark:bg-muted/40">
        <InputGroupInput
          id="home-agent-prompt"
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          placeholder={locale === 'ar' ? 'اسأل الوكيل عن يومك أو أي موضوع آخر…' : 'Ask Agent about today or anything else…'}
          aria-label={locale === 'ar' ? 'رسالة إلى الوكيل' : 'Message Agent'}
          dir="auto"
          className="h-11 min-w-0 px-2 text-sm"
          data-testid="input-home-agent-prompt"
        />
        <InputGroupAddon align="inline-end" className="gap-1 border-0 pe-1">
          <div
            className="agent-composer-actions"
            data-hidden={hasPrompt ? 'true' : undefined}
            aria-hidden={hasPrompt}
            inert={hasPrompt}
          >
            <InputGroupButton
              type="button"
              size="icon-sm"
              className="h-9 w-9 rounded-full"
              aria-label={locale === 'ar' ? 'فتح إدخال الصوت في الوكيل' : 'Open voice input in Agent'}
              title={locale === 'ar' ? 'إدخال صوتي' : 'Voice input'}
              onClick={continueInAgent}
              data-testid="button-home-voice-input"
            >
              <Mic className="size-5" aria-hidden="true" />
            </InputGroupButton>
            <InputGroupButton
              type="button"
              size="icon-sm"
              className="h-9 w-9 rounded-full"
              aria-label={locale === 'ar' ? 'فتح المحادثة الصوتية في الوكيل' : 'Open Voice Chat in Agent'}
              title={locale === 'ar' ? 'محادثة صوتية' : 'Voice Chat'}
              onClick={continueInAgent}
              data-testid="button-home-voice-chat"
            >
              <AudioLines className="size-5" aria-hidden="true" />
            </InputGroupButton>
            <InputGroupButton
              type="button"
              size="icon-sm"
              className="h-9 w-9 rounded-full"
              aria-label={locale === 'ar' ? 'فتح الوضع المباشر في الوكيل' : 'Open Live Mode in Agent'}
              title={locale === 'ar' ? 'الوضع المباشر' : 'Live Mode'}
              onClick={continueInAgent}
              data-testid="button-home-live-mode"
            >
              <PhoneCall className="size-4" aria-hidden="true" />
            </InputGroupButton>
          </div>
          <Button
            type="submit"
            size="icon"
            className="h-9 w-9 shrink-0 rounded-full"
            aria-label={locale === 'ar' ? 'إرسال الرسالة' : 'Send message'}
            data-testid="button-home-open-agent"
          >
             <ArrowUp className="size-5" aria-hidden="true" />
          </Button>
        </InputGroupAddon>
      </InputGroup>
    </form>
  );
}
