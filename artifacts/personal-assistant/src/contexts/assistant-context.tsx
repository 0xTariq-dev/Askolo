import { createContext, useContext, useState, ReactNode } from 'react';

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  id: string;
}

interface AssistantContextValue {
  isOpen: boolean;
  isFull: boolean;
  messages: ChatMessage[];
  open: () => void;
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

  const open = () => setIsOpen(true);
  const close = () => {
    setIsOpen(false);
    setIsFull(false);
  };
  const toggle = () => setIsOpen((prev) => !prev);
  const toggleFull = () => setIsFull((prev) => !prev);

  return (
    <AssistantContext.Provider
      value={{ isOpen, isFull, messages, open, close, toggle, toggleFull, setMessages }}
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
