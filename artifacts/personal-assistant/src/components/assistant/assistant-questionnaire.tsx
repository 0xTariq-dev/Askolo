import { useId, useState } from 'react';
import type { AssistantQuestionnaireResult } from '@workspace/api-client-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Textarea } from '@/components/ui/textarea';
import { useLocale } from '@/contexts/locale-context';

type Answer = string | string[];

export function AssistantQuestionnaire({
  questionnaire,
  disabled,
  onSubmit,
}: {
  questionnaire: AssistantQuestionnaireResult;
  disabled: boolean;
  onSubmit: (text: string) => void;
}) {
  const { locale } = useLocale();
  const instanceId = useId();
  const isArabic = locale === 'ar';
  const [answers, setAnswers] = useState<Record<string, Answer>>({});
  const [writeIns, setWriteIns] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [submitted, setSubmitted] = useState(false);

  const updateAnswer = (id: string, value: Answer) => {
    setAnswers((current) => ({ ...current, [id]: value }));
    setError('');
  };

  const submit = () => {
    if (disabled || submitted) return;
    const missing = questionnaire.questions.find((question) => {
      if (!question.required) return false;
      const value = answers[question.id];
      if (question.allowWriteIn && writeIns[question.id]?.trim()) return false;
      return question.type === 'checkbox'
        ? !Array.isArray(value) || value.length === 0
        : typeof value !== 'string' || !value.trim();
    });
    if (missing) {
      setError(isArabic ? 'أكمل الأسئلة المطلوبة قبل الإرسال.' : 'Answer the required questions before sending.');
      const firstControl = document.getElementById(`${instanceId}-answer-${missing.id}`)
        ?? document.querySelector<HTMLInputElement>(`input[name="answer-${missing.id}"]`);
      firstControl?.focus();
      return;
    }

    const response = questionnaire.questions.map((question, index) => {
      const answer = answers[question.id];
      const value = question.type === 'checkbox'
        ? Array.isArray(answer) ? answer.join(', ') : ''
        : question.allowWriteIn && writeIns[question.id]?.trim()
          ? writeIns[question.id].trim()
          : typeof answer === 'string' ? answer : '';
      return `${index + 1}. ${question.prompt}\n${isArabic ? 'الإجابة' : 'Answer'}: ${value || (isArabic ? 'تخطي' : 'Skipped')}`;
    }).join('\n\n');
    setSubmitted(true);
    onSubmit(response);
  };

  return (
    <section className="mt-3 space-y-4" aria-label={isArabic ? 'أسئلة المساعد' : 'Assistant questions'}>
      {questionnaire.questions.map((question, index) => {
        const selected = answers[question.id];
        const selectedValues = Array.isArray(selected) ? selected : [];
        return (
          <div key={question.id} className="space-y-2">
            <p className="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {isArabic ? `السؤال ${index + 1}` : `Question ${index + 1}`}
            </p>
            <Card className="border-border/80 bg-background/80 shadow-none">
              <CardHeader className="p-4 pb-2">
                <CardTitle id={`${instanceId}-question-${question.id}`} tabIndex={-1} className="text-sm leading-6">
                  {question.prompt}
                  {question.required && <span className="ms-1 text-destructive" aria-label={isArabic ? 'مطلوب' : 'required'}>*</span>}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 p-4 pt-2">
                {question.type === 'radio' && (
                  <fieldset className="space-y-2" aria-labelledby={`${instanceId}-question-${question.id}`}>
                    <legend className="sr-only">{question.prompt}</legend>
                    {question.options.map((option) => (
                      <label key={option} className="flex min-h-11 cursor-pointer items-center gap-3 rounded-xl border border-border/70 px-3 py-2 text-sm hover:bg-accent/50 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
                        <input
                          type="radio"
                          name={`answer-${question.id}`}
                          value={option}
                          checked={selected === option}
                          disabled={disabled || submitted}
                          onChange={() => updateAnswer(question.id, option)}
                          className="size-4 accent-primary"
                        />
                        <span>{option}</span>
                      </label>
                    ))}
                    {question.allowWriteIn && (
                      <Textarea
                        aria-label={isArabic ? 'أو اكتب إجابتك' : 'Or write your own answer'}
                        value={writeIns[question.id] ?? ''}
                        disabled={disabled || submitted}
                        onChange={(event) => {
                          setWriteIns((current) => ({ ...current, [question.id]: event.target.value }));
                          updateAnswer(question.id, '');
                        }}
                        placeholder={isArabic ? 'أو اكتب إجابتك' : 'Or write your own answer'}
                        className="min-h-16"
                      />
                    )}
                  </fieldset>
                )}
                {question.type === 'checkbox' && (
                  <fieldset className="space-y-2" aria-labelledby={`${instanceId}-question-${question.id}`}>
                    <legend className="sr-only">{question.prompt}</legend>
                    {question.options.map((option) => (
                      <label key={option} className="flex min-h-11 cursor-pointer items-center gap-3 rounded-xl border border-border/70 px-3 py-2 text-sm hover:bg-accent/50 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
                        <input
                          type="checkbox"
                          name={`answer-${question.id}`}
                          value={option}
                          checked={selectedValues.includes(option)}
                          disabled={disabled || submitted}
                          onChange={(event) => updateAnswer(
                            question.id,
                            event.target.checked
                              ? [...selectedValues, option]
                              : selectedValues.filter((value) => value !== option),
                          )}
                          className="size-4 accent-primary"
                        />
                        <span>{option}</span>
                      </label>
                    ))}
                  </fieldset>
                )}
                {question.type === 'text' && (
                  <Textarea
                    id={`${instanceId}-answer-${question.id}`}
                    aria-label={question.prompt}
                    value={typeof selected === 'string' ? selected : ''}
                    disabled={disabled || submitted}
                    onChange={(event) => updateAnswer(question.id, event.target.value)}
                    placeholder={isArabic ? 'اكتب إجابتك…' : 'Type your answer…'}
                    className="min-h-20"
                  />
                )}
              </CardContent>
            </Card>
          </div>
        );
      })}
      {error && <p className="text-sm text-destructive" role="alert">{error}</p>}
      <Button type="button" size="sm" className="min-h-10" disabled={disabled || submitted} onClick={submit}>
        {submitted
          ? (isArabic ? 'تم الإرسال' : 'Answers sent')
          : (isArabic ? 'إرسال الإجابات' : 'Send answers')}
      </Button>
    </section>
  );
}
