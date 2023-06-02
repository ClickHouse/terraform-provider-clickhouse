const bodyParser = require('body-parser')
const express = require('express')
const morgan = require('morgan')

const app = express()

const services = []

app.use(bodyParser.json())
app.use(morgan('dev'))

app.get('/services', (req, res) => {
  res.send(services)
})

app.post('/services', (req, res) => {
  const { service } = req.body

  console.log('Creating service', req.body)

  const newService = {
    ...service,
    id: `${services.length}`
  }
  services.push(newService)

  res.send(newService)
})

app.get('/services/:id', (req, res) => {
  const findIndex = services.findIndex((r) => r.id === req.params.id)

  if (findIndex === -1) {
    return res.status(404).send({
      error: 'Service Not Found'
    })
  }

  res.send(services[findIndex])
})

app.put('/services/:id', (req, res) => {
  const { service } = req.body

  const findIndex = services.findIndex((r) => r.id === req.params.id)

  if (findIndex === -1) {
    return res.status(404).send({
      error: 'Service Not Found'
    })
  }

  delete service.id

  services[findIndex] = {
    ...services[findIndex],
    ...service
  }

  res.send(services[findIndex])
})

app.delete('/services/:id', (req, res) => {
  const findIndex = services.findIndex((r) => r.id === req.params.id)

  if (findIndex === -1) {
    return res.status(404).send({
      error: 'Service Not Found'
    })
  }

  const service = services.splice(findIndex, 1)

  res.send(service)
})

app.listen(4444, () => {
  console.log('App listening on port 4444')
})
