# LocalWhatsAppMCP

LocalWhatsAppMCP connects a user's WhatsApp conversations to their chosen agents. This glossary separates the linked WhatsApp account, local conversation scope, and independently authorized agent connections.

## Language

**WhatsApp connection**:
The linked-device relationship between LocalWhatsAppMCP and the user's WhatsApp account.
_Avoid_: Agent connection, MCP pairing

**WhatsApp linking**:
The user scanning the displayed QR code to establish a WhatsApp connection.
_Avoid_: Agent pairing

**Initial capture**:
The setup period in which received history and messages are retained before the user confirms chat scope.
_Avoid_: Complete history import

**Available history**:
The historical messages actually supplied by WhatsApp and retained locally, which may cover only part of a conversation.

**Chat scope**:
The user's shared selection of conversations available for ongoing retention and agent access.
_Avoid_: Per-agent chat selection

**All chats**:
A chat-scope mode that includes existing and future direct chats and groups.

**Selected chats**:
A chat-scope mode containing only conversations explicitly included by the user.

**Chat catalog**:
The conversation identities and display information used to find and select chats, including conversations outside the current scope.

**Local archive**:
The retained message content and attachment references. It may contain content from earlier initial capture or a previous chat scope.

**Agent connection**:
An independently authorized MCP client connection with its own identity, credential, and sending permission.
_Avoid_: WhatsApp connection

**Pairing invitation**:
The generic setup instructions and temporary pairing code the user gives to an agent to establish one agent connection.

**Pairing code**:
A temporary secret that can be exchanged once for an agent connection's access token.
_Avoid_: Persistent access token

**Access token**:
The durable credential that authorizes an agent connection until its access is revoked.
_Avoid_: Pairing code, WhatsApp session

**Sending permission**:
An agent connection's optional permission to send new text and attachments to selected conversations.

**Local MCP service**:
The app-owned MCP server running on the user's Mac. Authorized agents reach it locally or through a user-managed ngrok tunnel; its credentials remain distinct from WhatsApp linking.

**Attachment reference**:
The recorded information identifying media associated with a message before or after the file is downloaded.

**Attachment cache**:
Downloaded media retained locally for reuse, still subject to the parent conversation's current access policy.
