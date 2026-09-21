import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useLocation } from 'wouter';
import {
  Camera,
  Calendar,
  Mail,
  CheckCircle2,
  XCircle,
  Loader2,
  AlertTriangle,
  LogOut,
  User,
  Key,
  Trash2,
  PlugZap,
  Unplug,
  ShieldCheck,
} from 'lucide-react';
import {
  useGetGoogleStatus,
  useGetTranscriptionPreferences,
  useUpdateTranscriptionPreferences,
  getGetTranscriptionPreferencesQueryKey,
} from '@workspace/api-client-react';
import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { useToast } from '@/hooks/use-toast';
import { isAppProductionHost, toPublicUrl } from '@/lib/site-domains';
import { useAppAuth } from '@/contexts/auth-context';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

async function apiDelete(path: string) {
  const res = await fetch(`${basePath}/api${path}`, { method: 'DELETE', credentials: 'include' });
  if (!res.ok && res.status !== 204) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `Request failed: ${res.status}`);
  }
}

export function ProfilePage() {
  const {
    user,
    isLoaded,
    authProvider,
    signOut,
    updateProfile,
    updateProfileImage,
    updatePassword,
  } = useAppAuth();
  const [, setLocation] = useLocation();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const { data: googleStatus, refetch: refetchGoogle, isFetching: checkingGoogle } =
    useGetGoogleStatus();
  const { data: voicePreferences } = useGetTranscriptionPreferences();
  const updateVoiceConsent = useUpdateTranscriptionPreferences();

  // Identity edit state
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [editingName, setEditingName] = useState(false);
  const [savingName, setSavingName] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [uploadingPhoto, setUploadingPhoto] = useState(false);

  // Password change state
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);
  const [passwordFormOpen, setPasswordFormOpen] = useState(false);

  // Independent recovery email state
  const [recoveryEmail, setRecoveryEmail] = useState('');
  const [recoveryReauthPassword, setRecoveryReauthPassword] = useState('');
  const [recoveryCode, setRecoveryCode] = useState('');
  const [recoveryStep, setRecoveryStep] = useState<'idle' | 'verify'>('idle');
  const [savingRecoveryEmail, setSavingRecoveryEmail] = useState(false);

  // MFA enrollment and recovery-code state. Secrets remain in transient component state only.
  const [mfaEnabled, setMfaEnabled] = useState(false);
  const [mfaStep, setMfaStep] = useState<'idle' | 'confirm' | 'codes'>('idle');
  const [mfaSecret, setMfaSecret] = useState('');
  const [mfaCode, setMfaCode] = useState('');
  const [mfaPassword, setMfaPassword] = useState('');
  const [mfaRecoveryCode, setMfaRecoveryCode] = useState('');
  const [mfaRecoveryCodes, setMfaRecoveryCodes] = useState<string[]>([]);
  const [mfaBusy, setMfaBusy] = useState(false);

  // Action states
  const [disconnecting, setDisconnecting] = useState<'calendar' | 'gmail' | null>(null);
  const [deletingData, setDeletingData] = useState(false);
  const [deletingAccount, setDeletingAccount] = useState(false);

  if (!isLoaded || !user) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  const displayName = [user.firstName, user.lastName].filter(Boolean).join(' ') || 'User';
  const email = user.email || '';
  const initials = user.firstName?.[0] || 'U';

  // ── Handlers ─────────────────────────────────────────────────────────────

  const startEditName = () => {
    setFirstName(user.firstName || '');
    setLastName(user.lastName || '');
    setEditingName(true);
  };

  const saveName = async () => {
    setSavingName(true);
    try {
      await updateProfile({ firstName: firstName.trim(), lastName: lastName.trim() });
      setEditingName(false);
      toast({ title: 'Name updated' });
    } catch {
      toast({ title: 'Failed to update name', variant: 'destructive' });
    } finally {
      setSavingName(false);
    }
  };

  const handlePhotoChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploadingPhoto(true);
    try {
      if (authProvider !== 'password') {
        toast({ title: 'Profile photo editing is not available for native provider accounts yet' });
        return;
      }
      await updateProfileImage(file);
      toast({ title: 'Photo updated' });
    } catch {
      toast({ title: 'Failed to upload photo', variant: 'destructive' });
    } finally {
      setUploadingPhoto(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  const changePassword = async () => {
    if (!newPassword || (authProvider === 'password' && !currentPassword)) return;
    setChangingPassword(true);
    try {
      await updatePassword(currentPassword, newPassword);
      setPasswordFormOpen(false);
      setCurrentPassword('');
      setNewPassword('');
      toast({ title: 'Password changed' });
    } catch (err: any) {
      toast({ title: err?.errors?.[0]?.message || 'Failed to change password', variant: 'destructive' });
    } finally {
      setChangingPassword(false);
    }
  };

  const enrollRecoveryEmail = async () => {
    if (!recoveryEmail || !recoveryReauthPassword) return;
    setSavingRecoveryEmail(true);
    try {
      const response = await fetch(`${basePath}/api/auth/recovery/email/enroll`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: recoveryEmail,
          currentPassword: recoveryReauthPassword,
        }),
      });
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;
      if (!response.ok) throw new Error(payload?.error || 'Recovery email setup failed');
      setRecoveryStep('verify');
      toast({ title: 'Check your recovery email', description: 'Enter the verification code to finish setup.' });
    } catch (err) {
      toast({
        title: err instanceof Error ? err.message : 'Recovery email setup failed',
        variant: 'destructive',
      });
    } finally {
      setSavingRecoveryEmail(false);
    }
  };

  const verifyRecoveryEmail = async () => {
    if (!recoveryEmail || recoveryCode.length !== 6) return;
    setSavingRecoveryEmail(true);
    try {
      const response = await fetch(`${basePath}/api/auth/recovery/email/verify`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: recoveryEmail, code: recoveryCode }),
      });
      const payload = (await response.json().catch(() => null)) as { error?: string } | null;
      if (!response.ok) throw new Error(payload?.error || 'Recovery email verification failed');
      setRecoveryStep('idle');
      setRecoveryEmail('');
      setRecoveryReauthPassword('');
      setRecoveryCode('');
      toast({ title: 'Recovery email verified' });
    } catch (err) {
      toast({
        title: err instanceof Error ? err.message : 'Recovery email verification failed',
        variant: 'destructive',
      });
    } finally {
      setSavingRecoveryEmail(false);
    }
  };

  useEffect(() => {
    if (!isLoaded || !user) return;
    fetch(`${basePath}/api/auth/mfa/status`, { credentials: 'include' })
      .then(async (response) => {
        if (!response.ok) return;
        const payload = (await response.json()) as { enabled?: boolean };
        setMfaEnabled(payload.enabled === true);
      })
      .catch(() => undefined);
  }, [isLoaded, user]);

  const postMFA = async (path: string, body: Record<string, string>) => {
    const response = await fetch(`${basePath}${path}`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const payload = (await response.json().catch(() => null)) as {
      error?: string;
      secret?: string;
      recoveryCodes?: string[];
    } | null;
    if (!response.ok) throw new Error(payload?.error || 'MFA request failed');
    return payload ?? {};
  };

  const startMFAEnrollment = async () => {
    if (!mfaPassword) return;
    setMfaBusy(true);
    try {
      const payload = await postMFA('/api/auth/mfa/enroll', { currentPassword: mfaPassword });
      setMfaSecret(payload.secret || '');
      setMfaCode('');
      setMfaStep('confirm');
      toast({ title: 'MFA enrollment started', description: 'Add the secret to your authenticator app, then enter its code.' });
    } catch (err) {
      toast({ title: err instanceof Error ? err.message : 'MFA enrollment failed', variant: 'destructive' });
    } finally {
      setMfaBusy(false);
    }
  };

  const confirmMFAEnrollment = async () => {
    if (mfaCode.length !== 6) return;
    setMfaBusy(true);
    try {
      const payload = await postMFA('/api/auth/mfa/confirm', { code: mfaCode });
      setMfaEnabled(true);
      setMfaSecret('');
      setMfaCode('');
      setMfaRecoveryCodes(payload.recoveryCodes || []);
      setMfaStep('codes');
      toast({ title: 'MFA enabled', description: 'Save your recovery codes before leaving this page.' });
    } catch (err) {
      toast({ title: err instanceof Error ? err.message : 'MFA confirmation failed', variant: 'destructive' });
    } finally {
      setMfaBusy(false);
    }
  };

  const regenerateMFARecoveryCodes = async () => {
    if (!mfaPassword || !mfaRecoveryCode) return;
    setMfaBusy(true);
    try {
      const payload = await postMFA('/api/auth/mfa/recovery-codes/regenerate', {
        currentPassword: mfaPassword,
        recoveryCode: mfaRecoveryCode,
      });
      setMfaPassword('');
      setMfaRecoveryCode('');
      setMfaRecoveryCodes(payload.recoveryCodes || []);
      setMfaStep('codes');
      toast({ title: 'Recovery codes regenerated', description: 'Your previous recovery codes no longer work.' });
    } catch (err) {
      toast({ title: err instanceof Error ? err.message : 'Recovery-code regeneration failed', variant: 'destructive' });
    } finally {
      setMfaBusy(false);
    }
  };

  const disableMFA = async () => {
    if (!mfaPassword || !mfaRecoveryCode) return;
    setMfaBusy(true);
    try {
      await postMFA('/api/auth/mfa/disable', {
        currentPassword: mfaPassword,
        recoveryCode: mfaRecoveryCode,
      });
      setMfaEnabled(false);
      setMfaPassword('');
      setMfaRecoveryCode('');
      toast({ title: 'MFA disabled' });
    } catch (err) {
      toast({ title: err instanceof Error ? err.message : 'MFA disable failed', variant: 'destructive' });
    } finally {
      setMfaBusy(false);
    }
  };

  const connectGoogle = (scope: 'calendar' | 'gmail') => {
    window.location.href = `${basePath}/api/google/gmail/connect?scope=${scope}&redirectTo=/profile`;
  };

  const disconnectGoogle = async (scope: 'calendar' | 'gmail') => {
    setDisconnecting(scope);
    try {
      await apiDelete(`/google/disconnect?scope=${scope}`);
      await refetchGoogle();
      toast({ title: `${scope === 'calendar' ? 'Google Calendar' : 'Gmail'} disconnected` });
    } catch {
      toast({ title: 'Failed to disconnect', variant: 'destructive' });
    } finally {
      setDisconnecting(null);
    }
  };

  const deleteData = async () => {
    setDeletingData(true);
    try {
      await apiDelete('/user/data');
      toast({ title: 'All data deleted' });
    } catch {
      toast({ title: 'Failed to delete data', variant: 'destructive' });
    } finally {
      setDeletingData(false);
    }
  };

  const deleteAccount = async () => {
    setDeletingAccount(true);
    try {
      await apiDelete('/user/account');
      await signOut({ redirectUrl: isAppProductionHost() ? toPublicUrl('/') : basePath || '/' });
    } catch {
      toast({ title: 'Failed to delete account', variant: 'destructive' });
      setDeletingAccount(false);
    }
  };

  const revokeVoiceConsent = () => {
    updateVoiceConsent.mutate(
      { data: { consent: false } },
      {
        onSuccess: (updated) => {
          toast({ title: 'Voice consent revoked' });
          queryClient.setQueryData(getGetTranscriptionPreferencesQueryKey(), updated);
        },
      },
    );
  };

  const calendarConnected = googleStatus?.calendarConnected ?? false;
  const gmailConnected = googleStatus?.gmailConnected ?? false;

  return (
    <PageTransition className="max-w-2xl mx-auto space-y-6">
      <header className="mb-2">
        <h1 className="text-2xl font-display font-bold tracking-tight">Profile</h1>
        <p className="text-sm text-muted-foreground mt-1">Manage your identity, connections, and account.</p>
      </header>

      {/* ── Identity ───────────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <User className="h-4 w-4 text-primary" /> Identity
          </CardTitle>
          <CardDescription>Your name, photo, and login credentials.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {/* Avatar */}
          <div className="flex items-center gap-4">
            <div className="relative">
              <Avatar className="h-16 w-16 border-2 border-border">
                <AvatarImage src={user.imageUrl || user.profileImageUrl || undefined} />
                <AvatarFallback className="bg-primary/20 text-primary text-xl">{initials}</AvatarFallback>
              </Avatar>
              <button
                onClick={() => fileInputRef.current?.click()}
                disabled={uploadingPhoto || authProvider !== 'password'}
                className="absolute -bottom-1 -right-1 h-6 w-6 rounded-full bg-primary text-primary-foreground flex items-center justify-center shadow hover:bg-primary/90 transition-colors"
                aria-label="Change photo"
              >
                {uploadingPhoto ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <Camera className="h-3 w-3" />
                )}
              </button>
            </div>
            <div>
              <p className="font-medium">{displayName}</p>
              <p className="text-sm text-muted-foreground">{email}</p>
            </div>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              className="hidden"
              disabled={authProvider !== 'password'}
              onChange={handlePhotoChange}
            />
          </div>

          {/* Name */}
          {editingName ? (
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <Label>First name</Label>
                  <Input value={firstName} onChange={(e) => setFirstName(e.target.value)} />
                </div>
                <div className="space-y-1.5">
                  <Label>Last name</Label>
                  <Input value={lastName} onChange={(e) => setLastName(e.target.value)} />
                </div>
              </div>
              <div className="flex gap-2">
                <Button size="sm" onClick={saveName} disabled={savingName}>
                  {savingName && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                  Save
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setEditingName(false)}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <Button variant="outline" size="sm" onClick={startEditName}>
              Edit name
            </Button>
          )}

          {/* Password */}
          {authProvider && (
            <div className="border-t border-border pt-4">
            <div className="flex items-center gap-2 mb-3">
              <Key className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm font-medium">Password</span>
            </div>
            {passwordFormOpen ? (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label>{authProvider === 'password' ? 'Current password' : 'Current password (optional)'}</Label>
                  <Input
                    type="password"
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    autoComplete="current-password"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label>New password</Label>
                  <Input
                    type="password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    autoComplete="new-password"
                  />
                </div>
                <div className="flex gap-2">
                  <Button size="sm" onClick={changePassword} disabled={changingPassword || !newPassword || (authProvider === 'password' && !currentPassword)}>
                    {changingPassword && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                    Change password
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setPasswordFormOpen(false)}>
                    Cancel
                  </Button>
                </div>
              </div>
            ) : (
              <Button variant="outline" size="sm" onClick={() => setPasswordFormOpen(true)}>
                Change password
              </Button>
            )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* ── Recovery email ────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="h-4 w-4 text-primary" /> Independent recovery
          </CardTitle>
          <CardDescription>
            Add a separate verified email for password recovery. Recent reauthentication is required.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {recoveryStep === 'verify' ? (
            <>
              <p className="text-sm text-muted-foreground">
                Enter the six-digit code sent to <span className="font-medium text-foreground">{recoveryEmail}</span>.
              </p>
              <div className="space-y-1.5">
                <Label htmlFor="recovery-code">Verification code</Label>
                <Input
                  id="recovery-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={recoveryCode}
                  onChange={(event) => setRecoveryCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
                  maxLength={6}
                  placeholder="123456"
                />
              </div>
              <div className="flex gap-2">
                <Button size="sm" onClick={verifyRecoveryEmail} disabled={savingRecoveryEmail || recoveryCode.length !== 6}>
                  {savingRecoveryEmail && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                  Verify recovery email
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setRecoveryStep('idle')} disabled={savingRecoveryEmail}>
                  Cancel
                </Button>
              </div>
            </>
          ) : (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="recovery-email">Recovery email</Label>
                <Input
                  id="recovery-email"
                  type="email"
                  autoComplete="email"
                  value={recoveryEmail}
                  onChange={(event) => setRecoveryEmail(event.target.value)}
                  placeholder="recovery@example.com"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="recovery-reauth-password">Current password</Label>
                <Input
                  id="recovery-reauth-password"
                  type="password"
                  autoComplete="current-password"
                  value={recoveryReauthPassword}
                  onChange={(event) => setRecoveryReauthPassword(event.target.value)}
                  placeholder="Confirm your current password"
                />
              </div>
              <Button
                size="sm"
                onClick={enrollRecoveryEmail}
                disabled={savingRecoveryEmail || !recoveryEmail || !recoveryReauthPassword}
              >
                {savingRecoveryEmail && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                Send verification code
              </Button>
            </>
          )}
        </CardContent>
      </Card>

      {/* ── Multi-factor authentication ───────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="h-4 w-4 text-primary" /> Multi-factor authentication
          </CardTitle>
          <CardDescription>
            Protect sign-in with an authenticator app. Recovery codes are shown once and cannot be restored.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {mfaStep === 'codes' ? (
            <>
              <p className="text-sm text-foreground font-medium">Save these recovery codes somewhere secure.</p>
              <p className="text-xs text-muted-foreground">
                Each code works once. Askolo will not show them again after you continue.
              </p>
              <div className="grid grid-cols-2 gap-2 rounded-lg border border-border bg-muted/30 p-3" aria-label="MFA recovery codes">
                {mfaRecoveryCodes.map((code) => (
                  <code key={code} className="text-sm font-mono text-foreground">{code}</code>
                ))}
              </div>
              <Button size="sm" onClick={() => { setMfaRecoveryCodes([]); setMfaStep('idle'); }}>
                I saved my recovery codes
              </Button>
            </>
          ) : !mfaEnabled && mfaStep === 'confirm' ? (
            <>
              <p className="text-sm text-muted-foreground">
                Add this setup key to your authenticator app. It is shown only during enrollment.
              </p>
              <code className="block break-all rounded-lg border border-border bg-muted/30 p-3 text-sm font-mono select-all">
                {mfaSecret}
              </code>
              <div className="space-y-1.5">
                <Label htmlFor="mfa-enrollment-code">Authenticator code</Label>
                <Input
                  id="mfa-enrollment-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={mfaCode}
                  onChange={(event) => setMfaCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
                  placeholder="123456"
                  maxLength={6}
                />
              </div>
              <div className="flex gap-2">
                <Button size="sm" onClick={confirmMFAEnrollment} disabled={mfaBusy || mfaCode.length !== 6}>
                  {mfaBusy && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                  Confirm MFA
                </Button>
                <Button size="sm" variant="ghost" onClick={() => { setMfaSecret(''); setMfaCode(''); setMfaStep('idle'); }} disabled={mfaBusy}>
                  Cancel
                </Button>
              </div>
            </>
          ) : !mfaEnabled ? (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="mfa-enrollment-password">Current password</Label>
                <Input
                  id="mfa-enrollment-password"
                  type="password"
                  autoComplete="current-password"
                  value={mfaPassword}
                  onChange={(event) => setMfaPassword(event.target.value)}
                />
              </div>
              <Button size="sm" onClick={startMFAEnrollment} disabled={mfaBusy || !mfaPassword}>
                {mfaBusy && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                Set up authenticator app
              </Button>
            </>
          ) : (
            <>
              <p className="text-sm text-green-400">MFA is enabled for this account.</p>
              <div className="space-y-1.5">
                <Label htmlFor="mfa-management-password">Current password</Label>
                <Input
                  id="mfa-management-password"
                  type="password"
                  autoComplete="current-password"
                  value={mfaPassword}
                  onChange={(event) => setMfaPassword(event.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="mfa-recovery-code">Recovery code</Label>
                <Input
                  id="mfa-recovery-code"
                  autoComplete="one-time-code"
                  value={mfaRecoveryCode}
                  onChange={(event) => setMfaRecoveryCode(event.target.value.toUpperCase())}
                  placeholder="ABCD-1234-5678-9ABC"
                />
              </div>
              <div className="flex flex-wrap gap-2">
                <Button size="sm" variant="outline" onClick={regenerateMFARecoveryCodes} disabled={mfaBusy || !mfaPassword || !mfaRecoveryCode}>
                  Regenerate recovery codes
                </Button>
                <Button size="sm" variant="outline" className="text-destructive border-destructive/30 hover:bg-destructive/10" onClick={disableMFA} disabled={mfaBusy || !mfaPassword || !mfaRecoveryCode}>
                  Disable MFA
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* ── Google Connections ─────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <PlugZap className="h-4 w-4 text-primary" /> Google Connections
          </CardTitle>
          <CardDescription>Connect your Google Calendar and Gmail to unlock smart features.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Calendar row */}
          <div className="flex items-center justify-between py-2">
            <div className="flex items-center gap-3">
              <div className="h-9 w-9 rounded-lg bg-blue-500/10 border border-blue-500/20 flex items-center justify-center">
                <Calendar className="h-4 w-4 text-blue-400" />
              </div>
              <div>
                <p className="text-sm font-medium">Google Calendar</p>
                <div className="flex items-center gap-1.5 mt-0.5">
                  {calendarConnected ? (
                    <>
                      <CheckCircle2 className="h-3.5 w-3.5 text-green-400" />
                      <span className="text-xs text-green-400">Connected</span>
                    </>
                  ) : (
                    <>
                      <XCircle className="h-3.5 w-3.5 text-muted-foreground" />
                      <span className="text-xs text-muted-foreground">Not connected</span>
                    </>
                  )}
                </div>
              </div>
            </div>
            {calendarConnected ? (
              <Button
                size="sm"
                variant="outline"
                className="text-destructive border-destructive/30 hover:bg-destructive/10"
                disabled={disconnecting === 'calendar'}
                onClick={() => disconnectGoogle('calendar')}
              >
                {disconnecting === 'calendar' ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
                ) : (
                  <Unplug className="h-3.5 w-3.5 mr-1.5" />
                )}
                Disconnect
              </Button>
            ) : (
              <Button size="sm" onClick={() => connectGoogle('calendar')}>
                Connect
              </Button>
            )}
          </div>

          <div className="border-t border-border" />

          {/* Gmail row */}
          <div className="flex items-center justify-between py-2">
            <div className="flex items-center gap-3">
              <div className="h-9 w-9 rounded-lg bg-red-500/10 border border-red-500/20 flex items-center justify-center">
                <Mail className="h-4 w-4 text-red-400" />
              </div>
              <div>
                <p className="text-sm font-medium">Gmail</p>
                <div className="flex items-center gap-1.5 mt-0.5">
                  {gmailConnected ? (
                    <>
                      <CheckCircle2 className="h-3.5 w-3.5 text-green-400" />
                      <span className="text-xs text-green-400">Connected</span>
                    </>
                  ) : (
                    <>
                      <XCircle className="h-3.5 w-3.5 text-muted-foreground" />
                      <span className="text-xs text-muted-foreground">Not connected</span>
                    </>
                  )}
                </div>
              </div>
            </div>
            {gmailConnected ? (
              <Button
                size="sm"
                variant="outline"
                className="text-destructive border-destructive/30 hover:bg-destructive/10"
                disabled={disconnecting === 'gmail'}
                onClick={() => disconnectGoogle('gmail')}
              >
                {disconnecting === 'gmail' ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
                ) : (
                  <Unplug className="h-3.5 w-3.5 mr-1.5" />
                )}
                Disconnect
              </Button>
            ) : (
              <Button size="sm" onClick={() => connectGoogle('gmail')}>
                Connect
              </Button>
            )}
          </div>

          {checkingGoogle && (
            <p className="text-xs text-muted-foreground flex items-center gap-1.5">
              <Loader2 className="h-3 w-3 animate-spin" /> Checking connections…
            </p>
          )}
        </CardContent>
      </Card>

      {/* ── Voice & AI privacy ─────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="h-4 w-4 text-primary" /> Voice &amp; AI privacy
          </CardTitle>
          <CardDescription>Review or revoke permission for voice transcription.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="space-y-1">
            <p className="text-sm font-medium">
              {voicePreferences?.consentGiven ? 'Voice transcription is enabled' : 'Voice transcription is not enabled'}
            </p>
            <p className="text-xs text-muted-foreground">
              Recordings are deleted after transcription, are not used to train models, and PII is redacted from AI interactions.
            </p>
          </div>
          {voicePreferences?.consentGiven && (
            <Button
              size="sm"
              variant="outline"
              onClick={revokeVoiceConsent}
              disabled={updateVoiceConsent.isPending}
              className="shrink-0"
            >
              {updateVoiceConsent.isPending ? 'Updating…' : 'Revoke consent'}
            </Button>
          )}
        </CardContent>
      </Card>

      {/* ── Data & Account ─────────────────────────────────────────────────── */}
      <Card className="border-destructive/30">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base text-destructive">
            <AlertTriangle className="h-4 w-4" /> Data & Account
          </CardTitle>
          <CardDescription>Destructive actions — these cannot be undone.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Delete data */}
          <div className="flex items-center justify-between py-2">
            <div>
              <p className="text-sm font-medium">Delete all my data</p>
              <p className="text-xs text-muted-foreground mt-0.5">
                Removes habits, goals, events, notes, and Google tokens. Your account remains.
              </p>
            </div>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button size="sm" variant="outline" className="text-destructive border-destructive/30 hover:bg-destructive/10 shrink-0 ml-4">
                  <Trash2 className="h-3.5 w-3.5 mr-1.5" />
                  Delete data
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete all your data?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This permanently deletes your habits, goals, calendar events, notes, chores, action items, daily plans, and Google connection tokens. Your Askolo account will remain. This cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction
                    className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                    onClick={deleteData}
                    disabled={deletingData}
                  >
                    {deletingData && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                    Yes, delete my data
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>

          <div className="border-t border-border" />

          {/* Delete account */}
          <div className="flex items-center justify-between py-2">
            <div>
              <p className="text-sm font-medium">Delete account</p>
              <p className="text-xs text-muted-foreground mt-0.5">
                Deletes all data and permanently removes your Askolo account.
              </p>
            </div>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button size="sm" variant="destructive" className="shrink-0 ml-4">
                  <LogOut className="h-3.5 w-3.5 mr-1.5" />
                  Delete account
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete your account?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This permanently deletes all your data AND your Askolo account. You will be signed out immediately and will not be able to sign in again. This cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction
                    className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                    onClick={deleteAccount}
                    disabled={deletingAccount}
                  >
                    {deletingAccount && <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />}
                    Yes, delete my account
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </CardContent>
      </Card>
    </PageTransition>
  );
}
