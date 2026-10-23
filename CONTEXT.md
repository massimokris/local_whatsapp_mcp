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
The generated setup instructions and one-time token the user gives to an agent to establish an agent connection.

**Sending permission**:
An agent connection's optional permission to send new text and attachments to selected conversations.

**Cloud access**:
An optional public route to the Mac's MCP service, allowing a hosted agent to use the same local archive and permissions.

**Attachment reference**:
The recorded information identifying media associated with a message before or after the file is downloaded.

**Attachment cache**:
Downloaded media retained locally for reuse, still subject to the parent conversation's current access policy.
