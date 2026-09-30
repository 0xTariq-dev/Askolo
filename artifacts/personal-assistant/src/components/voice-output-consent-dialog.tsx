import { ShieldCheck } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { useLocale } from '@/contexts/locale-context';

export function VoiceOutputConsentDialog({
  open,
  onOpenChange,
  onConfirm,
  saving,
  error,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  saving: boolean;
  error?: string;
}) {
  const { locale, direction } = useLocale();
  const arabic = locale === 'ar';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent dir={direction}>
        <DialogHeader>
          <DialogTitle>
            <ShieldCheck className="mr-2 inline h-5 w-5 text-primary" aria-hidden="true" />
            {arabic ? 'قبل استخدام الإخراج الصوتي عبر Azure' : 'Before using Azure speech output'}
          </DialogTitle>
          <DialogDescription>
            {arabic
              ? 'يرجى مراجعة كيفية معالجة نص ردود المساعد عند طلب قراءتها بصوت عالٍ.'
              : 'Please review how assistant response text is handled when you ask to hear it aloud.'}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-sm text-muted-foreground">
          <p>
            {arabic
              ? 'يرسل Askolo نص رد المساعد المحفوظ إلى Microsoft Azure Speech لإنشاء الصوت. يُعاد الصوت إلى متصفحك للتشغيل ولا يُحفظ في حساب Askolo. قد تعالج Microsoft النص وفقًا لشروط خدمة Azure Speech.'
              : 'Askolo sends the stored assistant response text to Microsoft Azure Speech to generate audio. The audio is returned to your browser for playback and is not saved in your Askolo account. Microsoft may process the text under the Azure Speech service terms.'}
          </p>
          <p>
            {arabic
              ? 'إذا تعذرت عملية Azure أو تشغيل الصوت، فقد يستخدم Askolo محرك النطق في متصفحك مرة واحدة لهذا الرد. اختيار «ليس الآن» لن يرسل النص إلى Azure ولن يبدأ النطق عبر الجهاز.'
              : 'If Azure synthesis or audio playback fails, Askolo may use your browser’s speech engine once for that response. Choosing “Not now” will not send the text to Azure or start device speech.'}
          </p>
          <p>
            {arabic
              ? 'بعد حفظ الموافقة، أغلق هذه النافذة ثم اختر «استمع» لبدء التشغيل. لن يبدأ الصوت تلقائيًا. يمكنك إلغاء الموافقة من إعدادات الخصوصية في أي وقت.'
              : 'After consent is saved, close this dialog and choose Listen again to start playback. Audio will not start automatically. You can revoke permission at any time in your privacy settings.'}
          </p>
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {arabic ? 'ليس الآن' : 'Not now'}
          </Button>
          <Button onClick={onConfirm} disabled={saving}>
            {saving
              ? arabic ? 'جارٍ الحفظ…' : 'Saving…'
              : arabic ? 'أوافق على الإخراج الصوتي' : 'Allow speech output'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}