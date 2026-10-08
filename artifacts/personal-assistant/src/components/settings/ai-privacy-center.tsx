import { useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  AIPrivacyPreferencesUpdateAssemblyAiLiveVoice,
  getGetAIPrivacyPreferencesQueryKey,
  getGetTranscriptionPreferencesQueryKey,
  getGetVoiceOutputPreferencesQueryKey,
  useGetAIPrivacyPreferences,
  useGetVoiceOutputPreferences,
  useUpdateAIPrivacyPreferences,
  useUpdateVoiceOutputPreferences,
  type AIPrivacyPreferencesUpdate,
} from '@workspace/api-client-react';
import { ShieldCheck } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useLocale } from '@/contexts/locale-context';
import { CURRENT_VOICE_OUTPUT_CONSENT_VERSION } from '@/lib/voice-output-consent';

type ConsentPurpose = 'recorded-voice' | 'assistant-processing' | 'assemblyai-live' | 'azure-speech';

const purposeCopy: Record<ConsentPurpose, { title: string; description: string }> = {
  'recorded-voice': {
    title: 'AssemblyAI voice input',
    description:
      'AssemblyAI receives recorded dictation and live Voice Chat audio only after on-device speech detection. In Voice Chat, each provider session closes at the utterance boundary; recognized text is submitted to the shared Assistant conversation without a review step. Askolo does not persist or replay live audio or provider transcripts during connection recovery. Assistant’s separate app-side redaction runs before text reaches its AI provider. AssemblyAI’s processing region and retention for live transcription are unverified. Assistant processing, Azure output, and Live Mode each require separate permission; redaction can miss sensitive details.',
  },
  'assistant-processing': {
    title: 'Assistant processing and app-side redaction',
    description:
      'When you submit text or start Voice Chat, Askolo sends your message or recognized utterance to its configured AI provider. Voice Chat submits each transcript without a review step. Before sending, Askolo filters detected contact, financial and account, government-ID, credential, birth-date, and other high-risk values in the app. Names and project or client labels remain only after you grant this separate consent. Redaction can miss sensitive details.',
  },
  'assemblyai-live': {
    title: 'AssemblyAI Live Mode',
    description:
      'AssemblyAI’s managed Voice Agent processes live audio and produces spoken replies. The processing location and EU-only processing are unverified; account-wide model-improvement opt-out and Voice Agent retention coverage have not been verified. Askolo requests provider soft-delete when a session ends, but that does not prove physical erasure. Development Live Mode may proceed with your separate permission while these provider checks remain pending.',
  },
  'azure-speech': {
    title: 'Azure speech output',
    description:
      'Voice Chat sends Assistant reply text to Microsoft Azure Speech in sentence-sized chunks and streams audio to your device; Askolo does not save that audio. Azure output requires this separate permission, alongside separate AssemblyAI microphone and Assistant-processing permissions. Microsoft may process text under Azure Speech terms; Askolo has not verified the processing location or retention for your Azure setup. If Azure or playback fails, the full text remains available to read or retry; Askolo will not silently switch to browser speech.',
  },
};

const purposeCopyArabic: Record<ConsentPurpose, { title: string; description: string }> = {
  'recorded-voice': {
    title: 'مدخلات الصوت عبر AssemblyAI',
    description:
      'يستقبل AssemblyAI صوت الإملاء المسجّل وصوت Voice Chat المباشر فقط بعد اكتشاف الكلام على الجهاز. في Voice Chat، يُغلق اتصال المزوّد عند نهاية كل جملة ويُرسل النص المتعرّف عليه إلى محادثة المساعد المشتركة دون خطوة للمراجعة. لا يحفظ Askolo الصوت المباشر ولا يعيد تشغيله أو يعيد نص المزوّد أثناء استعادة الاتصال. يعمل تنقيح المساعد المنفصل داخل التطبيق قبل إرسال النص إلى مزوّد الذكاء الاصطناعي. لم يتم التحقق من موقع المعالجة أو مدة الاحتفاظ لدى AssemblyAI في النسخ المباشر. تتطلب معالجة المساعد وإخراج Azure وLive Mode موافقات منفصلة؛ وقد لا يكتشف التنقيح كل التفاصيل الحساسة.',
  },
  'assistant-processing': {
    title: 'معالجة المساعد والتنقيح داخل التطبيق',
    description:
      'عند إرسال نص أو بدء Voice Chat، يرسل Askolo رسالتك أو النص المتعرّف عليه إلى مزوّد الذكاء الاصطناعي المهيأ. يرسل Voice Chat كل نص إلى المحادثة دون خطوة للمراجعة. قبل الإرسال، يرشّح Askolo القيم الحساسة التي يكتشفها، مثل بيانات الاتصال والمال والحسابات والهوية الحكومية وبيانات الدخول وتواريخ الميلاد. تبقى الأسماء وأسماء المشاريع أو العملاء فقط بعد منح هذه الموافقة المنفصلة. قد لا يكتشف التنقيح كل التفاصيل الحساسة.',
  },
  'assemblyai-live': {
    title: 'وضع AssemblyAI Live',
    description:
      'يعالج Voice Agent المُدار من AssemblyAI الصوت المباشر وينشئ ردودًا منطوقة. لم يتم التحقق من موقع المعالجة أو ضمان المعالجة داخل الاتحاد الأوروبي؛ كما لم يتم التحقق من إعداد منع تحسين النماذج على مستوى الحساب أو تغطية الاحتفاظ الخاصة بـVoice Agent. يطلب Askolo الحذف المنطقي لدى المزوّد عند انتهاء الجلسة، لكن ذلك لا يثبت الإزالة الفعلية. يمكن استخدام Live Mode في بيئة التطوير بموافقتك المنفصلة بينما تظل هذه الفحوص معلّقة.',
  },
  'azure-speech': {
    title: 'الإخراج الصوتي عبر Azure',
    description:
      'يرسل وضع المحادثة الصوتية نص رد المساعد إلى Microsoft Azure Speech على دفعات بحجم جملة، ويبث الصوت إلى جهازك؛ ولا يحفظ Askolo هذا الصوت. يتطلب إخراج Azure هذه الموافقة المنفصلة، إلى جانب موافقتين منفصلتين على ميكروفون AssemblyAI ومعالجة المساعد. قد تعالج Microsoft النص وفق شروط Azure Speech؛ ولم يتحقق Askolo من موقع المعالجة أو مدة الاحتفاظ لإعداد Azure لديك. إذا تعذر Azure أو التشغيل، يبقى النص الكامل متاحًا للقراءة أو إعادة المحاولة، ولن ينتقل Askolo تلقائيًا إلى نطق المتصفح.',
  },
};

export function AIPrivacyCenter() {
  const queryClient = useQueryClient();
  const privacyQuery = useGetAIPrivacyPreferences();
  const outputQuery = useGetVoiceOutputPreferences();
  const privacyMutation = useUpdateAIPrivacyPreferences();
  const outputMutation = useUpdateVoiceOutputPreferences();
  const [purpose, setPurpose] = useState<ConsentPurpose | null>(null);
  const [redactionConfirmed, setRedactionConfirmed] = useState(false);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  const privacy = privacyQuery.data;
  const output = outputQuery.data;
  const outputConsentCurrent = Boolean(
    output?.consentGiven && output.consentVersion === CURRENT_VOICE_OUTPUT_CONSENT_VERSION,
  );
  const openConsent = (nextPurpose: ConsentPurpose) => {
    setError('');
    setRedactionConfirmed(false);
    setPurpose(nextPurpose);
  };
  const closeConsent = (open: boolean) => {
    if (!open && !saving) {
      setPurpose(null);
    }
  };
  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: getGetAIPrivacyPreferencesQueryKey() }),
      queryClient.invalidateQueries({ queryKey: getGetTranscriptionPreferencesQueryKey() }),
      queryClient.invalidateQueries({ queryKey: getGetVoiceOutputPreferencesQueryKey() }),
    ]);
  };

  const updatePrivacy = async (body: AIPrivacyPreferencesUpdate) => {
    await privacyMutation.mutateAsync({ data: body });
    await refresh();
  };
  const updateOutput = async (body: { consent?: boolean; autoSpeakEnabled?: boolean }) => {
    await outputMutation.mutateAsync({ data: body });
    await refresh();
  };

  const confirmConsent = async () => {
    if (!purpose || (purpose === 'assistant-processing' && !redactionConfirmed)) return;
    setSaving(true);
    setError('');
    try {
      if (purpose === 'recorded-voice') {
        await updatePrivacy({ recordedVoiceInput: true });
      } else if (purpose === 'assistant-processing') {
        await updatePrivacy({ assistantProcessing: true, redactionLocation: 'app' });
      } else if (purpose === 'assemblyai-live') {
        await updatePrivacy({ assemblyAiLive: true });
      } else {
        await updateOutput({ consent: true, autoSpeakEnabled: false });
      }
      setPurpose(null);
    } catch {
      setError('Your privacy setting could not be saved. Please try again.');
    } finally {
      setSaving(false);
    }
  };

  const revokePrivacy = async (key: 'recordedVoiceInput' | 'assistantProcessing' | 'assemblyAiLive') => {
    setError('');
    try {
      await updatePrivacy({ [key]: false });
    } catch {
      setError('Your privacy setting could not be updated. Please try again.');
    }
  };
  const toggleOutputConsent = async () => {
    if (!outputConsentCurrent) {
      openConsent('azure-speech');
      return;
    }
    setError('');
    try {
      await updateOutput({ consent: false });
    } catch {
      setError('Azure speech consent could not be revoked. Please try again.');
    }
  };

  const statusBadge = (enabled: boolean) => (
    <Badge variant={enabled ? 'default' : 'secondary'}>{enabled ? 'Enabled' : 'Not enabled'}</Badge>
  );

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="h-4 w-4 text-primary" /> Voice &amp; AI privacy
          </CardTitle>
          <CardDescription>
            These permissions are independent. You can revoke them here at any time.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {(privacyQuery.isError || outputQuery.isError) && (
            <p role="alert" className="text-sm text-destructive">Privacy settings could not be loaded.</p>
          )}
          <section className="space-y-4" aria-label="AI and voice permissions">
            <PreferenceRow
              title="AssemblyAI voice input"
              description="Allows AssemblyAI to transcribe recorded voice input in Assistant, Planning, Notes, and uploads, plus utterance-scoped Voice Chat audio after on-device speech detection. Assistant processing and Azure output are separate permissions."
              enabled={Boolean(privacy?.recordedVoiceInputConsentGiven)}
              onEnable={() => openConsent('recorded-voice')}
              onRevoke={() => void revokePrivacy('recordedVoiceInput')}
              busy={privacyMutation.isPending}
              statusBadge={statusBadge}
            />
            <PreferenceRow
              title="Assistant processing"
              description="Allows submitted text and Voice Chat transcripts to reach Askolo’s AI provider after app-side redaction; Voice Chat submits transcripts without review. Workspace changes still require separate confirmation."
              enabled={Boolean(privacy?.assistantProcessingConsentGiven)}
              onEnable={() => openConsent('assistant-processing')}
              onRevoke={() => void revokePrivacy('assistantProcessing')}
              busy={privacyMutation.isPending}
              statusBadge={statusBadge}
            />
            <div className="border-t pt-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <h3 className="text-sm font-medium">Transcript redaction location</h3>
                  <p className="text-sm text-muted-foreground">
                    Only app-side filtering is available. Full transcripts remain editable; detected high-risk values are filtered before Assistant processing.
                  </p>
                  <p className="text-xs font-medium" role="status">
                    {privacy?.redactionLocation === 'app' ? 'App-side redaction selected' : 'Choose app-side redaction before enabling Assistant processing'}
                  </p>
                </div>
                {privacy?.redactionLocation !== 'app' && (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={privacyMutation.isPending || privacyQuery.isLoading}
                    onClick={() => void updatePrivacy({ redactionLocation: 'app' }).catch(() => setError('App-side redaction could not be selected. Please try again.'))}
                  >
                    Select app-side
                  </Button>
                )}
              </div>
            </div>
            <PreferenceRow
              title="AssemblyAI Live Mode"
              description="Separate permission for live microphone audio and managed spoken responses. Development Live Mode may proceed while provider privacy checks remain pending; your separate permission is still required."
              enabled={Boolean(privacy?.assemblyAiLiveConsentGiven)}
              onEnable={() => openConsent('assemblyai-live')}
              onRevoke={() => void revokePrivacy('assemblyAiLive')}
              busy={privacyMutation.isPending}
              statusBadge={(enabled) => (
                <Badge variant={enabled && privacy?.assemblyAiLiveAvailable ? 'default' : 'secondary'}>
                  {enabled
                    ? privacy?.assemblyAiLiveAvailable ? 'Enabled' : 'Permission saved; unavailable'
                    : 'Not enabled'}
                </Badge>
              )}
            />
            <div className="border-t pt-4">
              <div className="max-w-md space-y-2">
                <Label htmlFor="assemblyai-live-voice">Live Mode voice</Label>
                <p className="text-sm text-muted-foreground">
                  This saved voice is independent of your app language and Azure speech voice.
                </p>
                <Select
                  value={privacy?.assemblyAiLiveVoice ?? 'michael'}
                  onValueChange={(value) => {
                    const voice = Object.values(AIPrivacyPreferencesUpdateAssemblyAiLiveVoice)
                      .find((candidate) => candidate === value);
                    if (voice) {
                      void updatePrivacy({ assemblyAiLiveVoice: voice })
                        .catch(() => setError('The Live Mode voice could not be saved. Please try again.'));
                    }
                  }}
                  disabled={privacyQuery.isLoading || privacyMutation.isPending}
                >
                  <SelectTrigger id="assemblyai-live-voice" data-testid="select-assemblyai-live-voice">
                    <SelectValue placeholder="Choose a voice" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="michael">US English — Michael</SelectItem>
                    <SelectItem value="mary">US English — Mary</SelectItem>
                    <SelectItem value="paul">UK English — Paul</SelectItem>
                    <SelectItem value="vera">UK English — Vera</SelectItem>
                    <SelectItem value="giovanni">Italian — Giovanni</SelectItem>
                    <SelectItem value="lola">Spanish — Lola</SelectItem>
                    <SelectItem value="juergen">German — Juergen</SelectItem>
                    <SelectItem value="rafael">Portuguese — Rafael</SelectItem>
                    <SelectItem value="estelle">French — Estelle</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="border-t pt-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="text-sm font-medium">Azure speech output</h3>
                    {statusBadge(outputConsentCurrent)}
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Azure speech is used only in Voice Chat. Normal Chat remains text-only.
                  </p>
                  <Button
                    type="button"
                    size="sm"
                    variant="link"
                    className="h-auto p-0"
                    disabled={outputMutation.isPending}
                    onClick={() => void toggleOutputConsent()}
                  >
                    {outputConsentCurrent ? 'Revoke Azure speech consent' : 'Review Azure speech consent'}
                  </Button>
                </div>
              </div>
            </div>
          </section>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        </CardContent>
      </Card>

      <AIConsentDialog
        purpose={purpose}
        open={purpose !== null}
        redactionConfirmed={redactionConfirmed}
        saving={saving}
        error={error}
        onOpenChange={closeConsent}
        onRedactionConfirmedChange={setRedactionConfirmed}
        onConfirm={() => void confirmConsent()}
      />
    </>
  );
}

function PreferenceRow({
  title,
  description,
  enabled,
  onEnable,
  onRevoke,
  busy,
  statusBadge,
}: {
  title: string;
  description: string;
  enabled: boolean;
  onEnable: () => void;
  onRevoke: () => void;
  busy: boolean;
  statusBadge: (enabled: boolean) => ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 border-b pb-4 last:border-b-0 last:pb-0 sm:flex-row sm:items-start sm:justify-between">
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="text-sm font-medium">{title}</h3>
          {statusBadge(enabled)}
        </div>
        <p className="max-w-3xl text-sm text-muted-foreground">{description}</p>
      </div>
      <Button type="button" size="sm" variant="outline" disabled={busy} onClick={enabled ? onRevoke : onEnable}>
        {enabled ? 'Revoke permission' : 'Review permission'}
      </Button>
    </div>
  );
}

export function AIConsentDialog({
  purpose,
  open,
  redactionConfirmed,
  saving,
  error,
  onOpenChange,
  onRedactionConfirmedChange,
  onConfirm,
}: {
  purpose: ConsentPurpose | null;
  open: boolean;
  redactionConfirmed: boolean;
  saving: boolean;
  error?: string;
  onOpenChange: (open: boolean) => void;
  onRedactionConfirmedChange?: (confirmed: boolean) => void;
  onConfirm: () => void;
}) {
  const { locale, direction } = useLocale();
  const arabic = locale === 'ar';
  const copy = purpose
    ? arabic ? purposeCopyArabic[purpose] : purposeCopy[purpose]
    : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent dir={direction}>
        <DialogHeader>
          <DialogTitle>{copy?.title ?? (arabic ? 'إذن الخصوصية' : 'Privacy permission')}</DialogTitle>
          <DialogDescription>{copy?.description ?? ''}</DialogDescription>
        </DialogHeader>
        {purpose === 'assistant-processing' && (
          <div className="flex items-start gap-3 rounded-md border p-3">
            <Checkbox
              id="confirm-app-side-redaction"
              checked={redactionConfirmed}
              onCheckedChange={(checked) => onRedactionConfirmedChange?.(checked === true)}
            />
            <Label htmlFor="confirm-app-side-redaction" className="text-sm leading-relaxed">
              {arabic
                ? 'استخدم تنقيح النص داخل التطبيق قبل إرسال المحتوى إلى مزوّد المساعد.'
                : 'Use app-side transcript redaction before sending content to the Assistant provider.'}
            </Label>
          </div>
        )}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter>
          <Button type="button" variant="outline" disabled={saving} onClick={() => onOpenChange(false)}>
            {arabic ? 'ليس الآن' : 'Not now'}
          </Button>
          <Button
            type="button"
            disabled={saving || (purpose === 'assistant-processing' && !redactionConfirmed)}
            onClick={onConfirm}
          >
            {saving
              ? arabic ? 'جارٍ الحفظ…' : 'Saving…'
              : arabic ? 'أفهم وأتابع' : 'I understand and continue'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
