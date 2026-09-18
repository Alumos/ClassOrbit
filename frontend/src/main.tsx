import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { PageBoundary } from './PageBoundary'
import './styles.css'

createRoot(document.getElementById('root')!).render(<StrictMode><PageBoundary><App /></PageBoundary></StrictMode>)
