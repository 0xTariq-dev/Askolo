import { createRoot } from 'react-dom/client';

import App from './App';
import { pingAppwrite } from './lib/appwrite';

import './index.css';

pingAppwrite();

createRoot(document.getElementById('root')!).render(<App />);
