import common from './common'
import errors from './errors'
import auth from './auth'
import overview from './overview'
import nodes from './nodes'
import security from './security'
import appearance from './appearance'
import apps from './apps'
import certs from './certs'

const messages = {
    common,
    errors,
    auth,
    overview,
    nodes,
    security,
    appearance,
    apps,
    certs,
}

// Every language pack must match this shape exactly.
export type MessageSchema = typeof messages
export default messages
