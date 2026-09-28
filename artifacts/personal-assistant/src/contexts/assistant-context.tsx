import { createContext, useContext, useState, ReactNode } from 'react';
import type { AssistantMessage as AssistantApiMessage } from '@workspace/api-client-react';

export interface ChatMessage extends AssistantApiMessage {}

interface AssistantContextValue {
  isOpen: boolean;
  isFull: boolean;
  messages: ChatMessage[];
  draft: string;
  open: () => void;
  openWithDraft: (text: string) => void;
  clearDraft: () => void;
  close: () => void;
  toggle: () => void;
  toggleFull: () => void;
  setMessages: (msgs: ChatMessage[] | ((prev: ChatMessage[]) => ChatMessage[])) => void;
}

const AssistantContext = createContext<AssistantContextValue | null>(null);

const WELCOME: ChatMessage = {
  role: 'assistant',
  content: "Hi, I'm Askolo. Ask me about your day, schedule, emails, or how to balance your goals.",
  id: 'welcome',
};

export function AssistantProvider({ children }: { children: ReactNode }) {
  const [isOpen, setIsOpen] = useState(false);
  const [isFull, setIsFull] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([WELCOME]);
  const [draft, setDraft] = useState('');

  const open = () => setIsOpen(true);
  const openWithDraft = (text: string) => {
    setDraft(text);
    setIsOpen(true);
  };
  const clearDraft = () => setDraft('');
  const close = () => {
    setIsOpen(false);
    setIsFull(false);
  };
  const toggle = () => setIsOpen((prev) => !prev);
  const toggleFull = () => setIsFull((prev) => !prev);

  return (
    <AssistantContext.Provider
      value={{ isOpen, isFull, messages, draft, open, openWithDraft, clearDraft, close, toggle, toggleFull, setMessages }}
    >
      {children}
    </AssistantContext.Provider>
  );
}

export function useAssistantState() {
  const ctx = useContext(AssistantContext);
  if (!ctx) throw new Error('useAssistantState must be used within AssistantProvider');
  return ctx;
}
