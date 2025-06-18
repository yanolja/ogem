#!/usr/bin/env tsx
/**
 * Function calling example for the Ogem JavaScript SDK
 */

import { Ogem, Models, createUserMessage } from '../src';

async function main() {
  console.log('=== Ogem JavaScript SDK - Function Calling Example ===\n');

  const client = new Ogem({
    baseURL: 'http://localhost:8080',
    apiKey: 'your-api-key'
  });

  // Define a weather function
  const tools = [
    {
      type: 'function' as const,
      function: {
        name: 'get_weather',
        description: 'Get current weather for a location',
        parameters: {
          type: 'object',
          properties: {
            location: {
              type: 'string',
              description: 'City and state, e.g. San Francisco, CA'
            },
            unit: {
              type: 'string',
              enum: ['celsius', 'fahrenheit'],
              description: 'Temperature unit'
            }
          },
          required: ['location']
        }
      }
    }
  ];

  try {
    console.log('Asking about weather...\n');
    
    const response = await client.chat.completions.create({
      model: Models.GPT_4,
      messages: [
        createUserMessage("What's the weather like in New York?")
      ],
      tools,
      tool_choice: 'auto'
    });

    const choice = response.choices[0];
    
    if (choice.message.tool_calls && choice.message.tool_calls.length > 0) {
      const toolCall = choice.message.tool_calls[0];
      console.log(`Function called: ${toolCall.function.name}`);
      console.log(`Arguments: ${toolCall.function.arguments}`);
      
      // Parse the arguments
      try {
        const args = JSON.parse(toolCall.function.arguments);
        console.log('Parsed arguments:', args);
        
        // Simulate function execution
        const weatherResult = {
          location: args.location,
          temperature: 72,
          condition: 'Sunny',
          humidity: 65,
          unit: args.unit || 'fahrenheit'
        };
        
        console.log('Simulated weather result:', weatherResult);
        
      } catch (parseError) {
        console.error('Failed to parse function arguments:', parseError);
      }
    } else {
      console.log(`Direct response: ${choice.message.content}`);
    }
    
  } catch (error) {
    console.error('Error:', error);
  }
}

if (require.main === module) {
  main().catch(console.error);
}