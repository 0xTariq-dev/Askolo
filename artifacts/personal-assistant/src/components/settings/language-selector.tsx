import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@workspace/askolo-design-system/components/ui/card';
import { Label } from '@workspace/askolo-design-system/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@workspace/askolo-design-system/components/ui/select';
import { useLocale } from '@/contexts/locale-context';
import { isLocale } from '@/lib/locale';
import { useToast } from '@workspace/askolo-design-system/hooks/use-toast';

export function LanguageSelector() {
  const { locale, setLocale, t } = useLocale();
  const { toast } = useToast();

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t('profile.language')}</CardTitle>
        <CardDescription>{t('profile.languageDescription')}</CardDescription>
      </CardHeader>
      <CardContent className="max-w-sm space-y-2">
        <Label htmlFor="app-language">{t('common.language')}</Label>
        <Select
          value={locale}
          onValueChange={(value) => {
            if (isLocale(value)) {
              void setLocale(value).catch(() => {
                toast({
                  title: t('profile.languageSyncFailed'),
                  variant: 'destructive',
                });
              });
            }
          }}
        >
          <SelectTrigger id="app-language" data-testid="select-app-language">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="en">{t('common.english')}</SelectItem>
            <SelectItem value="ar">{t('common.arabic')}</SelectItem>
          </SelectContent>
        </Select>
      </CardContent>
    </Card>
  );
}