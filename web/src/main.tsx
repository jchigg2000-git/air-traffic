import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App.tsx'
import './index.css'

// Restore theme before first paint.
const saved = localStorage.getItem('at-theme')
if (saved === 'light') document.documentElement.classList.remove('dark')
else document.documentElement.classList.add('dark')

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { refetchInterval: 5000, staleTime: 2500, refetchOnWindowFocus: false, retry: 1 },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
