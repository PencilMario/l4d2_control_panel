import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {App} from './app/App'
import {SelfServiceVPKPage} from './app/SelfServiceVPKPage'
import {configureVPKUploadQueue, selfServiceVPKUploadConfiguration} from './vpk/uploadQueue'

const selfService = window.location.pathname === '/uploadvpk'
if (selfService) configureVPKUploadQueue(selfServiceVPKUploadConfiguration)
createRoot(document.getElementById('root')!).render(<StrictMode>{selfService ? <SelfServiceVPKPage/> : <App/>}</StrictMode>)
