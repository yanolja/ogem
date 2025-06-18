#!/usr/bin/env tsx
/**
 * Streaming chat completion example for the Ogem JavaScript SDK
 */

import { Ogem, Models, createSystemMessage, createUserMessage } from '../src';

async function main() {
  console.log('=== Ogem JavaScript SDK - Streaming Example ===\n');

  const client = new Ogem({
    baseURL: 'http://localhost:8080',
    apiKey: 'your-api-key'
  });

  try {
    console.log('Assistant: ');
    
    const stream = await client.chat.completions.create({
      model: Models.GPT_3_5_TURBO,
      messages: [
        createSystemMessage('You are a helpful assistant.'),
        createUserMessage('Write a short poem about TypeScript.')
      ],
      max_tokens: 200,
      temperature: 0.8,
      stream: true
    });

    let fullResponse = '';
    
    for await (const chunk of stream) {
      const content = chunk.choices[0]?.delta?.content;
      if (content) {
        process.stdout.write(content);
        fullResponse += content;
      }
      
      if (chunk.choices[0]?.finish_reason) {
        console.log(`\n\nFinished with reason: ${chunk.choices[0].finish_reason}`);
        break;
      }
    }

    console.log(`\nFull response length: ${fullResponse.length} characters`);
    
  } catch (error) {
    console.error('Error:', error);
  }
}

if (require.main === module) {
  main().catch(console.error);
}