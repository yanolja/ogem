#!/usr/bin/env tsx
/**
 * Basic usage example for the Ogem JavaScript SDK
 */

import { Ogem, Models, createSystemMessage, createUserMessage } from '../src';

async function main() {
  console.log('=== Ogem JavaScript SDK - Basic Example ===\n');

  // Create client
  const client = new Ogem({
    baseURL: 'http://localhost:8080',
    apiKey: 'your-api-key',
    debug: true
  });

  try {
    // Health check
    console.log('1. Health Check');
    console.log('-'.repeat(40));
    const health = await client.health();
    console.log(`Server status: ${health.status}`);
    console.log(`Version: ${health.version}`);
    console.log(`Uptime: ${health.uptime}\n`);

    // Basic chat completion
    console.log('2. Basic Chat Completion');
    console.log('-'.repeat(40));
    const response = await client.chat.completions.create({
      model: Models.GPT_3_5_TURBO,
      messages: [
        createSystemMessage('You are a helpful assistant.'),
        createUserMessage('What is the capital of France?')
      ],
      max_tokens: 100,
      temperature: 0.7
    });

    console.log(`Response: ${response.choices[0].message.content}`);
    console.log(`Tokens used: ${response.usage?.total_tokens}`);
    console.log(`Model: ${response.model}\n`);

    // List models
    console.log('3. Available Models');
    console.log('-'.repeat(40));
    const models = await client.models.list();
    console.log(`Found ${models.data.length} models:`);
    models.data.slice(0, 5).forEach(model => {
      console.log(`  - ${model.id} (by ${model.owned_by})`);
    });
    
  } catch (error) {
    console.error('Error:', error);
  }
}

if (require.main === module) {
  main().catch(console.error);
}