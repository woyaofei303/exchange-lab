/** Browser entry point and shared query-cache configuration. */
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import App from './App'
import './styles.css'

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 2, refetchOnWindowFocus: false } } })
const root = document.getElementById('root')
if (!root) throw new Error('Missing #root element')

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
)
