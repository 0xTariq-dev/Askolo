import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useLocale } from '@/contexts/locale-context';
<<<<<<< HEAD
import { isLocale, LOCALE_REGISTRY, SUPPORTED_LOCALES } from '@/lib/locale';
=======
import { isLocale } from '@/lib/locale';
>>>>>>> c7ea45a5fdbf4bb422d76b6a13e0d4dca631b40a
import { useToast } from '@/hooks/use-toast';

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
            {SUPPORTED_LOCALES.map((option) => (
              <SelectItem key={option} value={option}>
                {LOCALE_REGISTRY[option].displayName}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardContent>
    </Card>
  );
}